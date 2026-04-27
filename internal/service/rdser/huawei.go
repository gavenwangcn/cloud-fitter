package rdser

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hwrds "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3/model"
	hwiam "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3"
	iammodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3/model"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbrds"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbutilization"
	"github.com/cloud-fitter/cloud-fitter/internal/envtags"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweices"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweicloudregion"
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
		cli := hwiam.IamClientBuilder().WithRegion(huaweicloudregion.EndpointForService("iam", rName)).WithCredential(auth).Build()
		c := hwiam.NewIamClient(cli)
		request := new(iammodel.KeystoneListProjectsRequest)
		request.Name = &rName
		r, err := c.KeystoneListProjects(request)
		if err != nil || len(*r.Projects) == 0 {
			return nil, errors.Wrapf(err, "Huawei KeystoneListProjects regionName %s", rName)
		}
		projectId := (*r.Projects)[0].Id

		auth = basic.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).WithProjectId(projectId).Build()
		hcClient := hwrds.RdsClientBuilder().WithRegion(huaweicloudregion.EndpointForService("rds", rName)).WithCredential(auth).Build()
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
	huaweiCESNamespaceRDS       = "SYS.RDS"
	huaweiCESDimRDSInstance     = "rds_instance_id"
	huaweiCESMetricRDSCPU       = "rds001_cpu_util"
	huaweiCESMetricRDSMem       = "rds002_mem_util"
	metricsPerRdsInstance       = 2
)

func rdsUtilizationWindowProto(peak, avg, min float64, ok bool) *pbutilization.UtilizationWindow {
	if !ok {
		return &pbutilization.UtilizationWindow{Available: false}
	}
	return &pbutilization.UtilizationWindow{
		PeakPercent: peak, AvgPercent: avg, MinPercent: min, Available: true,
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

	ids := make([]string, 0, len(rdsList))
	for _, e := range rdsList {
		if e == nil || e.InstanceId == "" {
			continue
		}
		ids = append(ids, e.InstanceId)
	}
	if len(ids) == 0 {
		return
	}

	type agg struct {
		cpuPeak, cpuAvg, cpuMin float64
		cpuOK                 bool
		memPeak, memAvg, memMin float64
		memOK                 bool
	}
	m30 := make(map[string]agg, len(ids))
	m180 := make(map[string]agg, len(ids))

	for _, batch := range huaweices.ChunkInstanceIDs(ids, metricsPerRdsInstance, huaweices.MaxMetricsPerBatch) {
		q := make([]huaweices.MetricQuery, 0, len(batch)*2)
		for _, id := range batch {
			q = append(q,
				huaweices.MetricQuery{
					Namespace: huaweiCESNamespaceRDS, DimName: huaweiCESDimRDSInstance,
					DimValue: id, MetricName: huaweiCESMetricRDSCPU,
				},
				huaweices.MetricQuery{
					Namespace: huaweiCESNamespaceRDS, DimName: huaweiCESDimRDSInstance,
					DimValue: id, MetricName: huaweiCESMetricRDSMem,
				},
			)
		}
		if s30, e30 := huaweices.BatchQueryAverageSeries(ctx, cli, q, from30, toMs); e30 != nil {
			huaweices.LogBatchError("RDS", accountName, regionName, e30)
		} else {
			for _, id := range batch {
				pc, ac, mc, okc := huaweices.PeakAvgMinFromAveragePoints(s30[id+"\x00"+huaweiCESMetricRDSCPU])
				pm, am, mm, okm := huaweices.PeakAvgMinFromAveragePoints(s30[id+"\x00"+huaweiCESMetricRDSMem])
				m30[id] = agg{
					cpuPeak: pc, cpuAvg: ac, cpuMin: mc, cpuOK: okc,
					memPeak: pm, memAvg: am, memMin: mm, memOK: okm,
				}
			}
		}
		if s180, e180 := huaweices.BatchQueryAverageSeries(ctx, cli, q, from180, toMs); e180 != nil {
			huaweices.LogBatchError("RDS", accountName, regionName, e180)
		} else {
			for _, id := range batch {
				pc, ac, mc, okc := huaweices.PeakAvgMinFromAveragePoints(s180[id+"\x00"+huaweiCESMetricRDSCPU])
				pm, am, mm, okm := huaweices.PeakAvgMinFromAveragePoints(s180[id+"\x00"+huaweiCESMetricRDSMem])
				m180[id] = agg{
					cpuPeak: pc, cpuAvg: ac, cpuMin: mc, cpuOK: okc,
					memPeak: pm, memAvg: am, memMin: mm, memOK: okm,
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
			CpuLast_30D:  rdsUtilizationWindowProto(a30.cpuPeak, a30.cpuAvg, a30.cpuMin, a30.cpuOK),
			CpuLast_180D: rdsUtilizationWindowProto(a180.cpuPeak, a180.cpuAvg, a180.cpuMin, a180.cpuOK),
			MemLast_30D:  rdsUtilizationWindowProto(a30.memPeak, a30.memAvg, a30.memMin, a30.memOK),
			MemLast_180D: rdsUtilizationWindowProto(a180.memPeak, a180.memAvg, a180.memMin, a180.memOK),
		}
	}
}

func (r *HuaweiRds) ListDetail(ctx context.Context, req *pbrds.ListDetailReq) (*pbrds.ListDetailResp, error) {
	request := new(model.ListInstancesRequest)
	offset := (req.PageNumber - 1) * req.PageSize
	request.Offset = &offset
	limit := req.PageSize
	request.Limit = &limit

	resp, err := r.cli.ListInstances(request)
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
	rdses := make([]*pbrds.RdsInstance, len(instances))
	for k, v := range instances {
		engine := ""
		engineVersion := ""
		if v.Datastore != nil {
			engine = huaweiRdsDatastoreTypeString(v.Datastore.Type)
			engineVersion = v.Datastore.Version
		}
		cpu := int32(0)
		if v.Cpu != nil {
			if n, err := strconv.ParseInt(*v.Cpu, 10, 32); err == nil {
				cpu = int32(n)
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
		var tagPairs [][2]string
		for _, tg := range v.Tags {
			tagPairs = append(tagPairs, [2]string{tg.Key, tg.Value})
		}
		regionName := strings.TrimSpace(v.Region)
		if regionName == "" {
			regionName = r.region.GetName()
		}
		rdses[k] = &pbrds.RdsInstance{
			Provider:      pbtenant.CloudProvider_huawei,
			AccoutName:    r.tenanter.AccountName(),
			InstanceId:    v.Id,
			InstanceName:  v.Name,
			RegionName:    regionName,
			InstanceType:  formatHuaweiRDSInstanceMode(v),
			Engine:        engine,
			EngineVersion: engineVersion,
			InstanceClass: v.FlavorRef,
			Status:        v.Status,
			CreationTime:  v.Created,
			ExpireTime:    "",
			Cpu:           cpu,
			MemoryMb:      memMB,
			PublicIps:     pub,
			PrivateIps:    priv,
			VpcId:         v.VpcId,
			Port:          v.Port,
			ChargeType:    charge,
			EnvTagValue:   envtags.FromPairs(envtags.RDSKey(), tagPairs),
			NodeTagValue:  envtags.FromPairs(envtags.NodeTagKey(), tagPairs),
		}
	}

	isFinished := false
	if len(rdses) < int(req.PageSize) {
		isFinished = true
	}

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
