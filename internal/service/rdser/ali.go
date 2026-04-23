package rdser

import (
	"context"
	"strconv"
	"sync"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	alirds "github.com/aliyun/alibaba-cloud-sdk-go/services/rds"
	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbrds"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/envtags"
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

func (rds *AliRds) ListDetail(ctx context.Context, req *pbrds.ListDetailReq) (*pbrds.ListDetailResp, error) {
	request := alirds.CreateDescribeDBInstancesRequest()
	request.PageNumber = requests.NewInteger(int(req.PageNumber))
	request.PageSize = requests.NewInteger(int(req.PageSize))
	request.NextToken = req.NextToken
	resp, err := rds.cli.DescribeDBInstances(request)
	if err != nil {
		return nil, errors.Wrap(err, "Aliyun ListDetail error")
	}

	tagKey := envtags.RDSKey()
	var tagByInst map[string]string
	if tagKey != "" {
		tagByInst, err = envtags.AliRDSInstanceTagMap(rds.cli, tagKey)
		if err != nil {
			glog.Warningf("Aliyun RDS DescribeTags account=%s region=%s: %v", rds.tenanter.AccountName(), rds.region.GetName(), err)
			tagByInst = nil
		}
	}

	var rdses = make([]*pbrds.RdsInstance, len(resp.Items.DBInstance))
	for k, v := range resp.Items.DBInstance {
		ev := ""
		if tagByInst != nil {
			ev = tagByInst[v.DBInstanceId]
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
		rdses[k] = &pbrds.RdsInstance{
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
		}
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
