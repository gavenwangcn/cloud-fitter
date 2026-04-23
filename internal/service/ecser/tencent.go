package ecser

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbecs"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
)

type TencentCvm struct {
	cli      *cvm.Client
	region   tenanter.Region
	tenanter tenanter.Tenanter
}

func newTencentCvmClient(region tenanter.Region, tenant tenanter.Tenanter) (Ecser, error) {
	var (
		client *cvm.Client
		err    error
	)

	switch t := tenant.(type) {
	case *tenanter.AccessKeyTenant:
		client, err = cvm.NewClient(common.NewCredential(t.GetId(), t.GetSecret()), region.GetName(), profile.NewClientProfile())
	default:
	}

	if err != nil {
		return nil, errors.Wrap(err, "init tencent cvm client error")
	}
	return &TencentCvm{
		cli:      client,
		region:   region,
		tenanter: tenant,
	}, nil
}

func tencentDiskInfo(v *cvm.Instance) (sysGB, dataGB int32, summary string) {
	var parts []string
	if v.SystemDisk != nil && v.SystemDisk.DiskSize != nil {
		sysGB = int32(*v.SystemDisk.DiskSize)
		dt := ""
		if v.SystemDisk.DiskType != nil {
			dt = *v.SystemDisk.DiskType
		}
		parts = append(parts, fmt.Sprintf("系统盘:%dGB(%s)", sysGB, dt))
	}
	for _, dd := range v.DataDisks {
		if dd == nil || dd.DiskSize == nil {
			continue
		}
		sz := int32(*dd.DiskSize)
		dataGB += sz
		dt := ""
		if dd.DiskType != nil {
			dt = *dd.DiskType
		}
		parts = append(parts, fmt.Sprintf("数据盘:%dGB(%s)", sz, dt))
	}
	return sysGB, dataGB, strings.Join(parts, "; ")
}

func (ecs *TencentCvm) ListDetail(ctx context.Context, req *pbecs.ListDetailReq) (*pbecs.ListDetailResp, error) {
	request := cvm.NewDescribeInstancesRequest()
	request.Offset = common.Int64Ptr(int64((req.PageNumber - 1) * req.PageSize))
	request.Limit = common.Int64Ptr(int64(req.PageSize))
	resp, err := ecs.cli.DescribeInstances(request)
	if err != nil {
		return nil, errors.Wrap(err, "Tencent ListDetail error")
	}

	nInst := len(resp.Response.InstanceSet)
	glog.Infof("Tencent CVM ListDetail disk(from DescribeInstances) begin account=%s region=%s instances_in_page=%d page_number=%d",
		ecs.tenanter.AccountName(), ecs.region.GetName(), nInst, req.PageNumber)

	var ecses = make([]*pbecs.EcsInstance, nInst)
	for k, v := range resp.Response.InstanceSet {
		imageID := ""
		if v.ImageId != nil {
			imageID = *v.ImageId
		}
		osName := ""
		if v.OsName != nil {
			osName = *v.OsName
		}
		sysGB, dataGB, dsum := tencentDiskInfo(v)
		glog.V(2).Infof("Tencent CVM disk instance_id=%s account=%s region=%s sys_gb=%d data_gb=%d summary=%q",
			*v.InstanceId, ecs.tenanter.AccountName(), ecs.region.GetName(), sysGB, dataGB, dsum)
		ecses[k] = &pbecs.EcsInstance{
			Provider:         pbtenant.CloudProvider_tencent,
			AccountName:      ecs.tenanter.AccountName(),
			InstanceId:       *v.InstanceId,
			InstanceName:     *v.InstanceName,
			RegionName:       ecs.region.GetName(),
			PublicIps:        make([]string, len(v.PublicIpAddresses)),
			InstanceType:     *v.InstanceType,
			Cpu:              int32(*v.CPU),
			Memory:           int32(*v.Memory),
			Description:      "",
			Status:           *v.InstanceState,
			CreationTime:     *v.CreatedTime,
			ExpireTime:       *v.ExpiredTime,
			InnerIps:         make([]string, len(v.PrivateIpAddresses)),
			VpcId:            *v.VirtualPrivateCloud.VpcId,
			ResourceGroupId:  "",
			ChargeType:       *v.InstanceChargeType,
			ImageId:          imageID,
			ImageName:        osName,
			OsType:           "",
			OsBit:            "",
			SystemDiskSizeGb: sysGB,
			DataDiskTotalGb:  dataGB,
			DiskSummary:      dsum,
		}
		for k1, v1 := range v.PublicIpAddresses {
			ecses[k].PublicIps[k1] = *v1
		}
		for k1, v1 := range v.PrivateIpAddresses {
			ecses[k].InnerIps[k1] = *v1
		}
	}

	glog.Infof("Tencent CVM ListDetail disk end account=%s region=%s instances=%d (use -v=2 for per-instance lines)",
		ecs.tenanter.AccountName(), ecs.region.GetName(), nInst)

	isFinished := false
	if len(ecses) < int(req.PageSize) {
		isFinished = true
	}

	return &pbecs.ListDetailResp{
		Ecses:      ecses,
		Finished:   isFinished,
		NextToken:  "",
		PageNumber: req.PageNumber + 1,
		PageSize:   req.PageSize,
		RequestId:  *resp.Response.RequestId,
	}, nil
}
