package rdser

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	alirds "github.com/aliyun/alibaba-cloud-sdk-go/services/rds"
	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbrds"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/envtags"
	"github.com/cloud-fitter/cloud-fitter/internal/server/scope"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

var aliClientMutex sync.Mutex

type AliRds struct {
	cli      *alirds.Client
	region   tenanter.Region
	tenanter tenanter.Tenanter
}

func newAliRdsClient(region tenanter.Region, tenant tenanter.Tenanter) (Rdser, error) {
	var (
		client *alirds.Client
		err    error
	)

	switch t := tenant.(type) {
	case *tenanter.AccessKeyTenant:
		// 阿里云的sdk有一个 map 的并发问题，go test 加上-race 能检测出来，所以这里加一个锁
		aliClientMutex.Lock()
		client, err = alirds.NewClientWithAccessKey(region.GetName(), t.GetId(), t.GetSecret())
		aliClientMutex.Unlock()
	default:
	}

	if err != nil {
		return nil, errors.Wrap(err, "init ali rds client error")
	}

	return &AliRds{
		cli:      client,
		region:   region,
		tenanter: tenant,
	}, nil
}

func aliRDSSecurityGroupNames(cli *alirds.Client, dbInstanceID string) []string {
	req := alirds.CreateDescribeSecurityGroupConfigurationRequest()
	req.DBInstanceId = dbInstanceID
	resp, err := cli.DescribeSecurityGroupConfiguration(req)
	if err != nil {
		glog.Warningf("Aliyun DescribeSecurityGroupConfiguration instance=%s: %v", dbInstanceID, err)
		return nil
	}
	var out []string
	for _, rel := range resp.Items.EcsSecurityGroupRelation {
		if nm := strings.TrimSpace(rel.SecurityGroupName); nm != "" {
			out = append(out, nm)
			continue
		}
		if id := strings.TrimSpace(rel.SecurityGroupId); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func (rds *AliRds) ListDetail(ctx context.Context, req *pbrds.ListDetailReq) (*pbrds.ListDetailResp, error) {
	request := alirds.CreateDescribeDBInstancesRequest()
	request.PageNumber = requests.NewInteger(int(req.PageNumber))
	request.PageSize = requests.NewInteger(int(req.PageSize))
	request.NextToken = req.NextToken
	resp, err := rds.cli.DescribeDBInstances(request)
	if err != nil {
		return nil, errors.Wrap(err, "Aliyun ListDetail error")
	}

	envKey := envtags.RDSKey()
	nodeKey := envtags.NodeTagKey()
	sysKey := envtags.SystemTagKey()
	var tagByInst, nodeByInst, systemByInst map[string]string
	if envKey != "" || nodeKey != "" || sysKey != "" {
		tagByInst, nodeByInst, systemByInst, err = envtags.AliRDSInstanceTagValues(rds.cli, envKey, nodeKey, sysKey)
		if err != nil {
			glog.Warningf("Aliyun RDS DescribeTags account=%s region=%s: %v", rds.tenanter.AccountName(), rds.region.GetName(), err)
			tagByInst, nodeByInst, systemByInst = nil, nil, nil
		}
	}

	var rdses []*pbrds.RdsInstance
	for _, v := range resp.Items.DBInstance {
		ev := ""
		if tagByInst != nil {
			ev = tagByInst[v.DBInstanceId]
		}
		nv := ""
		if nodeByInst != nil {
			nv = nodeByInst[v.DBInstanceId]
		}
		sv := ""
		if systemByInst != nil {
			sv = systemByInst[v.DBInstanceId]
		}
		if !scope.SystemListTagFilterMatches(ctx, sv) {
			continue
		}
		instName := v.DBInstanceName
		if instName == "" {
			instName = v.DBInstanceDescription
		}
		st := v.DBInstanceStatus
		if st == "" {
			st = v.Status
		}
		cpu := int32(0)
		if v.DBInstanceCPU != "" {
			if n, err := strconv.ParseInt(v.DBInstanceCPU, 10, 32); err == nil {
				cpu = int32(n)
			}
		}
		memMB := int32(v.DBInstanceMemory)
		rdses = append(rdses, &pbrds.RdsInstance{
			Provider:      pbtenant.CloudProvider_ali,
			AccoutName:    rds.tenanter.AccountName(),
			InstanceId:    v.DBInstanceId,
			InstanceName:  instName,
			RegionName:    rds.region.GetName(),
			InstanceType:  v.DBInstanceType,
			Engine:        v.Engine,
			EngineVersion: v.EngineVersion,
			InstanceClass: v.DBInstanceClass,
			Status:        st,
			CreationTime:  v.CreateTime,
			ExpireTime:    v.ExpireTime,
			Cpu:           cpu,
			MemoryMb:      memMB,
			VpcId:         v.VpcId,
			ChargeType:    v.PayType,
			EnvTagValue:   ev,
			NodeTagValue:  nv,
		})
	}

	n := len(rdses)
	if n > 0 {
		var wg sync.WaitGroup
		sem := make(chan struct{}, 10)
		for i := 0; i < n; i++ {
			i := i
			id := rdses[i].InstanceId
			if id == "" {
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				rdses[i].SecurityGroupNames = aliRDSSecurityGroupNames(rds.cli, id)
			}()
		}
		wg.Wait()
	}

	// PageNumber 分页：无 NextToken，以本页条数判断；NextToken 分页：以返回 NextToken 是否为空为准（末页可能仍满页）
	isFinished := resp.NextToken == "" && (len(rdses) < int(req.PageSize) || req.NextToken != "")

	return &pbrds.ListDetailResp{
		Rdses:      rdses,
		Finished:   isFinished,
		PageNumber: req.PageNumber + 1,
		PageSize:   req.PageSize,
		NextToken:  resp.NextToken,
		RequestId:  resp.RequestId,
	}, nil
}
