package ecser

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	aliecs "github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbecs"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/envtags"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

var aliClientMutex sync.Mutex

type AliEcs struct {
	cli      *aliecs.Client
	region   tenanter.Region
	tenanter tenanter.Tenanter
}

func newAliEcsClient(region tenanter.Region, tenant tenanter.Tenanter) (Ecser, error) {
	var (
		client *aliecs.Client
		err    error
	)

	switch t := tenant.(type) {
	case *tenanter.AccessKeyTenant:
		// 阿里云的sdk有一个 map 的并发问题，go test 加上-race 能检测出来，所以这里加一个锁
		aliClientMutex.Lock()
		client, err = aliecs.NewClientWithAccessKey(region.GetName(), t.GetId(), t.GetSecret())
		aliClientMutex.Unlock()
	default:
	}

	if err != nil {
		return nil, errors.Wrap(err, "init ali ecs client error")
	}

	return &AliEcs{
		cli:      client,
		region:   region,
		tenanter: tenant,
	}, nil
}

func (ecs *AliEcs) aliDiskStats(instanceId string) (sysGB, dataGB int32, summary string, err error) {
	var sys int32
	var dataSum int32
	var parts []string
	page := 1
	for {
		dreq := aliecs.CreateDescribeDisksRequest()
		dreq.InstanceId = instanceId
		dreq.PageNumber = requests.NewInteger(page)
		dreq.PageSize = requests.NewInteger(50)
		dresp, derr := ecs.cli.DescribeDisks(dreq)
		if derr != nil {
			glog.Warningf("Aliyun DescribeDisks failed instance_id=%s account=%s region=%s page=%d err=%v",
				instanceId, ecs.tenanter.AccountName(), ecs.region.GetName(), page, derr)
			return 0, 0, "", derr
		}
		disks := dresp.Disks.Disk
		for _, d := range disks {
			switch d.Type {
			case "system":
				sys = int32(d.Size)
				parts = append(parts, fmt.Sprintf("系统盘:%dGB(%s)", d.Size, d.Category))
			case "data":
				dataSum += int32(d.Size)
				parts = append(parts, fmt.Sprintf("数据盘:%dGB(%s)", d.Size, d.Category))
			default:
				parts = append(parts, fmt.Sprintf("%s:%dGB(%s)", d.Type, d.Size, d.Category))
			}
		}
		if len(disks) < 50 {
			break
		}
		page++
	}
	sum := strings.Join(parts, "; ")
	glog.V(2).Infof("Aliyun DescribeDisks ok instance_id=%s account=%s region=%s sys_gb=%d data_gb=%d summary=%q pages=%d",
		instanceId, ecs.tenanter.AccountName(), ecs.region.GetName(), sys, dataSum, sum, page)
	return sys, dataSum, sum, nil
}

// aliPrivateIPs 对齐新版 DescribeInstances：优先 InnerIpAddress，空时从 eni 取 PrimaryIpAddress。
func aliPrivateIPs(v aliecs.Instance) []string {
	ips := append([]string(nil), v.InnerIpAddress.IpAddress...)
	if len(ips) > 0 {
		return ips
	}
	for _, ni := range v.NetworkInterfaces.NetworkInterface {
		if ni.PrimaryIpAddress != "" {
			ips = append(ips, ni.PrimaryIpAddress)
		}
	}
	return ips
}

// aliPublicIPs 合并公网 IP 与 EIP（去重）。
func aliPublicIPs(v aliecs.Instance) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(s string) {
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, ip := range v.PublicIpAddress.IpAddress {
		add(ip)
	}
	add(v.InternetIp)
	add(v.EipAddress.IpAddress)
	return out
}

func (ecs *AliEcs) ListDetail(ctx context.Context, req *pbecs.ListDetailReq) (*pbecs.ListDetailResp, error) {
	aliClientMutex.Lock()
	defer aliClientMutex.Unlock()

	request := aliecs.CreateDescribeInstancesRequest()
	request.PageNumber = requests.NewInteger(int(req.PageNumber))
	request.PageSize = requests.NewInteger(int(req.PageSize))
	request.NextToken = req.NextToken
	resp, err := ecs.cli.DescribeInstances(request)
	if err != nil {
		return nil, errors.Wrap(err, "Aliyun ListDetail error")
	}

	nInst := len(resp.Instances.Instance)
	glog.Infof("Aliyun ECS ListDetail DescribeDisks batch begin account=%s region=%s instances_in_page=%d page_number=%d",
		ecs.tenanter.AccountName(), ecs.region.GetName(), nInst, req.PageNumber)

	var ecses = make([]*pbecs.EcsInstance, nInst)
	var diskErrN int32
	for k, v := range resp.Instances.Instance {
		sysGB, dataGB, dsum, derr := ecs.aliDiskStats(v.InstanceId)
		if derr != nil {
			diskErrN++
			sysGB, dataGB, dsum = 0, 0, ""
			glog.Infof("Aliyun disk empty for instance_id=%s after DescribeDisks error", v.InstanceId)
		}
		tagPairs := make([][2]string, 0, len(v.Tags.Tag))
		for _, t := range v.Tags.Tag {
			tagPairs = append(tagPairs, [2]string{t.TagKey, t.TagValue})
		}
		cpu := int32(v.Cpu)
		if cpu == 0 {
			cpu = int32(v.CPU)
		}
		imgName := v.OSName
		if imgName == "" {
			imgName = v.OSNameEn
		}
		ecses[k] = &pbecs.EcsInstance{
			Provider:          pbtenant.CloudProvider_ali,
			AccountName:       ecs.tenanter.AccountName(),
			InstanceId:        v.InstanceId,
			InstanceName:      v.InstanceName,
			RegionName:        ecs.region.GetName(),
			PublicIps:         aliPublicIPs(v),
			InstanceType:      v.InstanceType,
			Cpu:               cpu,
			Memory:            int32(v.Memory),
			Description:       v.Description,
			Status:            v.Status,
			CreationTime:      v.CreationTime,
			ExpireTime:        v.ExpiredTime,
			InnerIps:          aliPrivateIPs(v),
			VpcId:             v.VpcAttributes.VpcId,
			ResourceGroupId:   v.ResourceGroupId,
			ChargeType:        v.InstanceChargeType,
			ImageId:           v.ImageId,
			ImageName:         imgName,
			OsType:            v.OSType,
			OsBit:             "",
			SystemDiskSizeGb:  sysGB,
			DataDiskTotalGb:   dataGB,
			DiskSummary:       dsum,
			EnvTagValue:       envtags.FromPairs(envtags.ECSKey(), tagPairs),
		}
	}

	glog.Infof("Aliyun ECS ListDetail DescribeDisks batch end account=%s region=%s disk_api_errors=%d/%d (use -v=2 for per-instance disk lines)",
		ecs.tenanter.AccountName(), ecs.region.GetName(), diskErrN, nInst)

	isFinished := resp.NextToken == "" && (len(ecses) < int(req.PageSize) || req.NextToken != "")

	return &pbecs.ListDetailResp{
		Ecses:      ecses,
		Finished:   isFinished,
		PageNumber: req.PageNumber + 1,
		PageSize:   req.PageSize,
		NextToken:  resp.NextToken,
		RequestId:  resp.RequestId,
	}, nil
}
