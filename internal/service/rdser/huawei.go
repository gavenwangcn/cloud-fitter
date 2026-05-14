package rdser

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hwiam "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3"
	hwrds "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3/model"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbrds"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbutilization"
	"github.com/cloud-fitter/cloud-fitter/internal/envtags"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweitags"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweices"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweicloudhelper"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweicloudregion"
	"github.com/cloud-fitter/cloud-fitter/internal/server/scope"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

type HuaweiRds struct {
	cli      *hwrds.RdsClient
	region   tenanter.Region
	tenanter tenanter.Tenanter
}

func newHuaweiRdsClient(region tenanter.Region, tenant tenanter.Tenanter) (Rdser, error) {
	var (
		client *hwrds.RdsClient
		err    error
	)

	switch t := tenant.(type) {
	case *tenanter.AccessKeyTenant:
		auth := basic.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).Build()
		rName := region.GetName()
		cli := hwiam.IamClientBuilder().WithRegion(huaweicloudregion.EndpointForService("iam", rName)).WithCredential(auth).WithHttpConfig(huaweicloudregion.SDKHttpConfig()).Build()
		c := hwiam.NewIamClient(cli)
		r, err := huaweicloudregion.KeystoneListProjectsResolveProject(c, rName)
		if err != nil || r == nil || r.Projects == nil || len(*r.Projects) == 0 {
			if err == nil {
				err = errors.New("empty project list")
			}
			return nil, errors.Wrapf(err, "Huawei KeystoneListProjects regionName %s", rName)
		}
		projectId := (*r.Projects)[0].Id

		auth = basic.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).WithProjectId(projectId).Build()
		hcClient := hwrds.RdsClientBuilder().WithRegion(huaweicloudregion.EndpointForService("rds", rName)).WithCredential(auth).WithHttpConfig(huaweicloudregion.SDKHttpConfig()).Build()
		client = hwrds.NewRdsClient(hcClient)
	default:
	}

	if err != nil {
		return nil, errors.Wrap(err, "init huawei rds client error")
	}
	return &HuaweiRds{
		cli:      client,
		region:   region,
		tenanter: tenant,
	}, nil
}

// fillHuaweiRDSSecurityGroupDisplayNames 将 ListInstances 返回的安全组 UUID 解析为 VPC 中的可读名称（失败则保留 ID）。
func fillHuaweiRDSSecurityGroupDisplayNames(region tenanter.Region, tenant tenanter.Tenanter, rdses []*pbrds.RdsInstance) {
	if len(rdses) == 0 {
		return
	}
	var idList []string
	for _, e := range rdses {
		if e == nil || len(e.SecurityGroupNames) != 1 {
			continue
		}
		id := strings.TrimSpace(e.SecurityGroupNames[0])
		if id != "" {
			idList = append(idList, id)
		}
	}
	if len(idList) == 0 {
		return
	}
	vpcCli, err := huaweicloudhelper.NewVPCClient(region, tenant)
	if err != nil {
		glog.Warningf("Huawei RDS VPC client for security group names: %v", err)
		return
	}
	nameByID := huaweicloudhelper.LookupSecurityGroupDisplayNames(vpcCli, idList)
	for _, e := range rdses {
		if e == nil || len(e.SecurityGroupNames) != 1 {
			continue
		}
		id := strings.TrimSpace(e.SecurityGroupNames[0])
		if nm, ok := nameByID[id]; ok {
			e.SecurityGroupNames = []string{nm}
		}
	}
}

func huaweiRdsDatastoreTypeString(t model.DatastoreType) string {
	b, err := json.Marshal(t)
	if err != nil {
		return ""
	}
	return strings.Trim(string(b), `"`)
}

// formatHuaweiRDSInstanceMode 对应控制台「实例类型」：ListInstances 的 type（Single/Ha/Replica）；
// 主备实例可附带 ha.replication_mode（async/semisync/sync）。
func formatHuaweiRDSInstanceMode(v model.InstanceResponse) string {
	t := strings.TrimSpace(v.Type)
	if v.Ha == nil {
		return t
	}
	b, err := json.Marshal(v.Ha.ReplicationMode)
	if err != nil {
		return t
	}
	rm := strings.Trim(string(b), `"`)
	if rm == "" {
		return t
	}
	return t + " (" + rm + ")"
}

const (
	huaweiCESNamespaceRDS  = "SYS.RDS"
	huaweiCESMetricRDSCPU  = "rds001_cpu_util"
	huaweiCESMetricRDSMem  = "rds002_mem_util"
	huaweiCESMetricRDSDisk = "rds039_disk_util"
	metricsPerRdsInstance  = 3
)

// huaweiRDSCESDimName 与华为云 CES「SYS.RDS」文档一致：不同引擎维度名不同，统一用 rds_instance_id 会查不到数据。
// 参考：RDS 监控指标说明（MySQL：rds_cluster_id；PostgreSQL：postgresql_cluster_id；SQL Server：rds_instance_sqlserver_id）。
func huaweiRDSCESDimName(engine string) string {
	e := strings.ToLower(strings.TrimSpace(engine))
	switch {
	case strings.Contains(e, "mysql"), strings.Contains(e, "mariadb"):
		return "rds_cluster_id"
	case strings.Contains(e, "postgresql"), strings.Contains(e, "postgres"):
		// 与 RDS API 附录「postgresql_cluster_id：RDS for PostgreSQL DB instance ID」一致；若仍无数据可再核对控制台指标维度。
		return "postgresql_cluster_id"
	case strings.Contains(e, "sqlserver"), strings.Contains(e, "sql server"):
		return "rds_instance_sqlserver_id"
	default:
		// 空引擎或未知类型：与 MySQL 最常见写法一致（ListInstances 的 id 对应 rds_cluster_id）
		return "rds_cluster_id"
	}
}

func rdsUtilizationWindowProto(peak, avg float64, ok bool) *pbutilization.UtilizationWindow {
	if !ok {
		return &pbutilization.UtilizationWindow{Available: false}
	}
	return &pbutilization.UtilizationWindow{
		PeakPercent: huaweices.RoundPercent2(peak),
		AvgPercent:  huaweices.RoundPercent2(avg),
		Available:   true,
	}
}

func rdsPeriodUtilizationRateProto(util float64, ok bool) *pbutilization.PeriodUtilizationRate {
	if !ok {
		return &pbutilization.PeriodUtilizationRate{Available: false}
	}
	return &pbutilization.PeriodUtilizationRate{
		UtilizationPercent: huaweices.RoundPercent2(util), Available: true,
	}
}

func fillHuaweiRDSUtilization(ctx context.Context, rdsList []*pbrds.RdsInstance, regionName string, tenant tenanter.Tenanter, accountName string) {
	if len(rdsList) == 0 {
		return
	}
	cli, err := huaweices.NewClient(regionName, tenant)
	if err != nil {
		glog.Warningf("Huawei RDS CES client init failed account=%s region=%s err=%v", accountName, regionName, err)
		return
	}
	now := time.Now().UTC()
	toMs := now.UnixMilli()
	from30 := now.Add(-30 * 24 * time.Hour).UnixMilli()
	from180 := now.Add(-180 * 24 * time.Hour).UnixMilli()

	filtered := make([]*pbrds.RdsInstance, 0, len(rdsList))
	for _, e := range rdsList {
		if e == nil || e.InstanceId == "" {
			continue
		}
		filtered = append(filtered, e)
	}
	if len(filtered) == 0 {
		return
	}

	type agg struct {
		cpuPeak, cpuAvg float64
		cpuOK           bool
		memPeak, memAvg float64
		memOK           bool
		diskUtil        float64
		diskOK          bool
	}
	m30 := make(map[string]agg, len(filtered))
	m180 := make(map[string]agg, len(filtered))

	perBatch := huaweices.MaxMetricsPerBatch / metricsPerRdsInstance
	if perBatch < 1 {
		perBatch = 1
	}
	for i := 0; i < len(filtered); i += perBatch {
		j := i + perBatch
		if j > len(filtered) {
			j = len(filtered)
		}
		batch := filtered[i:j]
		q := make([]huaweices.MetricQuery, 0, len(batch)*metricsPerRdsInstance)
		for _, e := range batch {
			dim := huaweiRDSCESDimName(e.Engine)
			id := e.InstanceId
			q = append(q,
				huaweices.MetricQuery{
					Namespace: huaweiCESNamespaceRDS, DimName: dim,
					DimValue: id, MetricName: huaweiCESMetricRDSCPU,
				},
				huaweices.MetricQuery{
					Namespace: huaweiCESNamespaceRDS, DimName: dim,
					DimValue: id, MetricName: huaweiCESMetricRDSMem,
				},
				huaweices.MetricQuery{
					Namespace: huaweiCESNamespaceRDS, DimName: dim,
					DimValue: id, MetricName: huaweiCESMetricRDSDisk,
				},
			)
		}
		if s30, e30 := huaweices.BatchQueryAverageSeries(ctx, cli, q, from30, toMs); e30 != nil {
			huaweices.LogBatchError("RDS", accountName, regionName, e30)
		} else {
			for _, e := range batch {
				id := e.InstanceId
				pc, ac, okc := huaweices.PeakAvgFromAveragePoints(s30[id+"\x00"+huaweiCESMetricRDSCPU])
				pm, am, mok := huaweices.PeakAvgFromAveragePoints(s30[id+"\x00"+huaweiCESMetricRDSMem])
				du, dok := huaweices.AvgFromAveragePoints(s30[id+"\x00"+huaweiCESMetricRDSDisk])
				m30[id] = agg{
					cpuPeak: pc, cpuAvg: ac, cpuOK: okc,
					memPeak: pm, memAvg: am, memOK: mok,
					diskUtil: du, diskOK: dok,
				}
			}
		}
		if s180, e180 := huaweices.BatchQueryAverageSeries(ctx, cli, q, from180, toMs); e180 != nil {
			huaweices.LogBatchError("RDS", accountName, regionName, e180)
		} else {
			for _, e := range batch {
				id := e.InstanceId
				pc, ac, okc := huaweices.PeakAvgFromAveragePoints(s180[id+"\x00"+huaweiCESMetricRDSCPU])
				pm, am, mok := huaweices.PeakAvgFromAveragePoints(s180[id+"\x00"+huaweiCESMetricRDSMem])
				du, dok := huaweices.AvgFromAveragePoints(s180[id+"\x00"+huaweiCESMetricRDSDisk])
				m180[id] = agg{
					cpuPeak: pc, cpuAvg: ac, cpuOK: okc,
					memPeak: pm, memAvg: am, memOK: mok,
					diskUtil: du, diskOK: dok,
				}
			}
		}
	}

	for _, e := range rdsList {
		if e == nil || e.InstanceId == "" {
			continue
		}
		a30 := m30[e.InstanceId]
		a180 := m180[e.InstanceId]
		e.UtilizationAudit = &pbutilization.ComputeUtilizationAudit{
			CpuLast_30D:   rdsUtilizationWindowProto(a30.cpuPeak, a30.cpuAvg, a30.cpuOK),
			CpuLast_180D:  rdsUtilizationWindowProto(a180.cpuPeak, a180.cpuAvg, a180.cpuOK),
			MemLast_30D:   rdsUtilizationWindowProto(a30.memPeak, a30.memAvg, a30.memOK),
			MemLast_180D:  rdsUtilizationWindowProto(a180.memPeak, a180.memAvg, a180.memOK),
			DiskLast_30D:  rdsPeriodUtilizationRateProto(a30.diskUtil, a30.diskOK),
			DiskLast_180D: rdsPeriodUtilizationRateProto(a180.diskUtil, a180.diskOK),
		}
	}
}

func (r *HuaweiRds) ListDetail(ctx context.Context, req *pbrds.ListDetailReq) (*pbrds.ListDetailResp, error) {
	request := new(model.ListInstancesRequest)
	offset := (req.PageNumber - 1) * req.PageSize
	request.Offset = &offset
	limit := req.PageSize
	request.Limit = &limit

	resp, err := huaweicloudregion.DoWithTransientNetworkRetry(func() (*model.ListInstancesResponse, error) {
		return r.cli.ListInstances(request)
	})
	if err != nil {
		return nil, errors.Wrap(err, "Huawei RDS ListInstances error")
	}

	if resp.Instances == nil {
		return &pbrds.ListDetailResp{
			Rdses:      nil,
			Finished:   true,
			PageNumber: req.PageNumber + 1,
			PageSize:   req.PageSize,
			NextToken:  "",
			RequestId:  "",
		}, nil
	}

	instances := *resp.Instances
	n := len(instances)
	type rdsTagFetch struct {
		apiPairs [][2]string
		sysPairs [][2]string
		usrPairs [][2]string
		err      error
	}
	tagFetches := make([]rdsTagFetch, n)
	var tagOK, tagFail int32
	glog.Infof("Huawei RDS ListDetail ListInstanceTags pull begin account=%s region=%s instances_in_page=%d",
		r.tenanter.AccountName(), r.region.GetName(), n)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)
	for ti := 0; ti < n; ti++ {
		ti := ti
		inst := instances[ti]
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			tr, err := huaweicloudregion.DoWithTransientNetworkRetry(func() (*model.ListInstanceTagsResponse, error) {
				return r.cli.ListInstanceTags(&model.ListInstanceTagsRequest{InstanceId: inst.Id})
			})
			if err != nil {
				atomic.AddInt32(&tagFail, 1)
				tagFetches[ti].err = err
				glog.Warningf("Huawei RDS ListInstanceTags failed instance_id=%s instance_name=%q account=%s region=%s err=%v (需 IAM 含查询实例标签相关只读权限，如 rds:instance:listInstanceTags)",
					inst.Id, inst.Name, r.tenanter.AccountName(), r.region.GetName(), err)
				return
			}
			atomic.AddInt32(&tagOK, 1)
			sysP, usrP := huaweitags.SplitRDSInstanceTags(tr.Tags)
			tagFetches[ti].apiPairs = huaweitags.PairsFromRDSListInstanceTags(tr.Tags)
			tagFetches[ti].sysPairs = sysP
			tagFetches[ti].usrPairs = usrP
		}()
	}
	wg.Wait()
	glog.Infof("Huawei RDS ListDetail ListInstanceTags pull end account=%s region=%s ok=%d fail=%d",
		r.tenanter.AccountName(), r.region.GetName(), tagOK, tagFail)

	huaweiRDSTagPairsFromList := func(tags []model.TagResponse) [][2]string {
		var out [][2]string
		for _, tg := range tags {
			out = append(out, [2]string{tg.Key, tg.Value})
		}
		return out
	}

	var rdses []*pbrds.RdsInstance
	for i, v := range instances {
		engine := ""
		engineVersion := ""
		if v.Datastore != nil {
			engine = huaweiRdsDatastoreTypeString(v.Datastore.Type)
			engineVersion = v.Datastore.Version
		}
		cpu := int32(0)
		if v.Cpu != nil {
			if n64, err := strconv.ParseInt(*v.Cpu, 10, 32); err == nil {
				cpu = int32(n64)
			}
		}
		memMB := int32(0)
		if v.Mem != nil {
			if gb, err := strconv.ParseFloat(*v.Mem, 64); err == nil {
				memMB = int32(gb * 1024)
			}
		}
		charge := ""
		if v.ChargeInfo != nil {
			raw, err := json.Marshal(v.ChargeInfo.ChargeMode)
			if err == nil {
				charge = strings.Trim(string(raw), `"`)
			}
		}
		pub := append([]string(nil), v.PublicIps...)
		priv := append([]string(nil), v.PrivateIps...)
		listPairs := huaweiRDSTagPairsFromList(v.Tags)
		var merged [][2]string
		if tagFetches[i].err == nil && len(tagFetches[i].apiPairs) > 0 {
			merged = huaweitags.MergePairsPreferPrimary(tagFetches[i].apiPairs, listPairs)
		} else {
			merged = listPairs
		}
		userPairsDisp := huaweitags.MergeRDSUserDisplayPairs(tagFetches[i].sysPairs, tagFetches[i].usrPairs, listPairs)
		if tagFetches[i].err != nil {
			userPairsDisp = huaweitags.MergeRDSUserDisplayPairs(nil, nil, listPairs)
		}
		if !scope.SystemListTagFilterMatches(ctx, envtags.FromPairs(envtags.SystemTagKey(), merged)) {
			continue
		}
		regionName := strings.TrimSpace(v.Region)
		if regionName == "" {
			regionName = r.region.GetName()
		}
		var sgNames []string
		if s := strings.TrimSpace(v.SecurityGroupId); s != "" {
			sgNames = []string{s}
		}
		rdses = append(rdses, &pbrds.RdsInstance{
			Provider:           pbtenant.CloudProvider_huawei,
			AccoutName:         r.tenanter.AccountName(),
			InstanceId:         v.Id,
			InstanceName:       v.Name,
			RegionName:         regionName,
			InstanceType:       formatHuaweiRDSInstanceMode(v),
			Engine:             engine,
			EngineVersion:      engineVersion,
			InstanceClass:      v.FlavorRef,
			Status:             v.Status,
			CreationTime:       v.Created,
			ExpireTime:         "",
			Cpu:                cpu,
			MemoryMb:           memMB,
			PublicIps:          pub,
			PrivateIps:         priv,
			VpcId:              v.VpcId,
			Port:               v.Port,
			ChargeType:         charge,
			SecurityGroupNames: sgNames,
			EnvTagValue: envtags.EnvTagOrNameFallback(
				envtags.FromPairs(envtags.RDSKey(), merged), v.Name),
			NodeTagValue: envtags.NodeTagOrNameFallback(
				envtags.FromPairs(envtags.NodeTagKey(), merged), v.Name),
			SystemTagsDisplay:    strings.TrimSpace(envtags.FromPairs(envtags.SystemTagKey(), merged)),
			UserTagsDisplay:      huaweitags.FormatPairsDisplay(userPairsDisp),
		})
	}

	isFinished := false
	if len(rdses) < int(req.PageSize) {
		isFinished = true
	}

	fillHuaweiRDSSecurityGroupDisplayNames(r.region, r.tenanter, rdses)
	fillHuaweiRDSUtilization(ctx, rdses, r.region.GetName(), r.tenanter, r.tenanter.AccountName())

	return &pbrds.ListDetailResp{
		Rdses:      rdses,
		Finished:   isFinished,
		PageNumber: req.PageNumber + 1,
		PageSize:   req.PageSize,
		NextToken:  "",
		RequestId:  "",
	}, nil
}
