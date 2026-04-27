package ecser

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hwecs "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	hwiam "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3"
	iammodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3/model"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbecs"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbutilization"
	"github.com/cloud-fitter/cloud-fitter/internal/envtags"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweices"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweicloudregion"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

type HuaweiEcs struct {
	cli      *hwecs.EcsClient
	region   tenanter.Region
	tenanter tenanter.Tenanter
}

func newHuaweiEcsClient(region tenanter.Region, tenant tenanter.Tenanter) (Ecser, error) {
	var (
		client *hwecs.EcsClient
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
		hcEcs := hwecs.EcsClientBuilder().WithRegion(huaweicloudregion.EndpointForService("ecs", rName)).WithCredential(auth).Build()
		client = hwecs.NewEcsClient(hcEcs)
	default:
	}

	if err != nil {
		return nil, errors.Wrap(err, "init huawei ecs client error")
	}
	return &HuaweiEcs{
		cli:      client,
		region:   region,
		tenanter: tenant,
	}, nil
}

func huaweiServerAddrIPType(a model.ServerAddress) string {
	if a.OSEXTIPStype == nil {
		return ""
	}
	raw, err := json.Marshal(a.OSEXTIPStype)
	if err != nil {
		return ""
	}
	return strings.Trim(string(raw), `"`)
}

func huaweiCollectIPs(addrs map[string][]model.ServerAddress, accessIPv4 string) (publicIPs, privateIPs []string) {
	pubSeen := make(map[string]struct{})
	privSeen := make(map[string]struct{})
	addPub := func(ip string) {
		if ip == "" {
			return
		}
		if _, ok := pubSeen[ip]; ok {
			return
		}
		pubSeen[ip] = struct{}{}
		publicIPs = append(publicIPs, ip)
	}
	addPriv := func(ip string) {
		if ip == "" {
			return
		}
		if _, ok := privSeen[ip]; ok {
			return
		}
		privSeen[ip] = struct{}{}
		privateIPs = append(privateIPs, ip)
	}
	for _, list := range addrs {
		for _, a := range list {
			switch huaweiServerAddrIPType(a) {
			case "floating":
				addPub(a.Addr)
			case "fixed":
				addPriv(a.Addr)
			default:
				if a.Addr != "" {
					addPriv(a.Addr)
				}
			}
		}
	}
	addPub(accessIPv4)
	return
}

func huaweiFlavorVCPU(f *model.ServerFlavor) int32 {
	if f == nil {
		return 0
	}
	n, err := strconv.ParseInt(f.Vcpus, 10, 32)
	if err != nil {
		return 0
	}
	return int32(n)
}

func huaweiFlavorRAMMB(f *model.ServerFlavor) int32 {
	if f == nil {
		return 0
	}
	n, err := strconv.ParseInt(f.Ram, 10, 32)
	if err != nil {
		return 0
	}
	return int32(n)
}

func huaweiFlavorName(f *model.ServerFlavor) string {
	if f == nil {
		return ""
	}
	return f.Name
}

func huaweiMetadataVPC(m map[string]string) string {
	if m == nil {
		return ""
	}
	return m["vpc_id"]
}

func huaweiMetadataChargeType(m map[string]string) string {
	if m == nil {
		return ""
	}
	switch m["charging_mode"] {
	case "0":
		return "postPaid"
	case "1":
		return "prePaid"
	case "2":
		return "spot"
	default:
		return m["charging_mode"]
	}
}

func huaweiECSImageID(v *model.ServerDetail) string {
	if v.Image != nil && v.Image.Id != "" {
		return v.Image.Id
	}
	if v.Metadata != nil {
		if id := v.Metadata["metering.image_id"]; id != "" {
			return id
		}
	}
	return ""
}

func huaweiMetadataGet(m map[string]string, key string) string {
	if m == nil {
		return ""
	}
	return m[key]
}

// huaweiDiskFromListServerBlockDevices 使用 ECS 磁盘管理 OpenAPI：
// 「查询弹性云服务器挂载磁盘列表详情信息」ListServerBlockDevices
// GET /v1/{project_id}/cloudservers/{server_id}/block_device
// 响应 volumeAttachments 含 size（GB）、bootIndex（0 为系统盘）。见华为云 API 文档 ecs 磁盘管理。
func huaweiDiskFromListServerBlockDevices(block *model.ListServerBlockDevicesResponse, flavor *model.ServerFlavor) (sysGB, dataGB int32, summary string) {
	var parts []string
	var sys int32
	var dataSum int32
	if block != nil && block.VolumeAttachments != nil {
		for _, b := range *block.VolumeAttachments {
			sz := int32(0)
			if b.Size != nil {
				sz = *b.Size
			}
			dev := ""
			if b.Device != nil {
				dev = *b.Device
			}
			isSys := b.BootIndex != nil && *b.BootIndex == 0
			if isSys {
				if sz > sys {
					sys = sz
				}
				if dev != "" {
					parts = append(parts, fmt.Sprintf("系统盘:%dGB(%s)", sz, dev))
				} else {
					parts = append(parts, fmt.Sprintf("系统盘:%dGB", sz))
				}
			} else {
				dataSum += sz
				if dev != "" {
					parts = append(parts, fmt.Sprintf("数据盘:%dGB(%s)", sz, dev))
				} else {
					parts = append(parts, fmt.Sprintf("数据盘:%dGB", sz))
				}
			}
		}
	}
	if sys == 0 && flavor != nil && flavor.Disk != "" {
		if n, err := strconv.ParseInt(flavor.Disk, 10, 32); err == nil && n > 0 {
			sys = int32(n)
		}
	}
	return sys, dataSum, strings.Join(parts, "; ")
}

const (
	huaweiCESNamespaceECS   = "SYS.ECS"
	huaweiCESDimECSInstance = "instance_id"
	huaweiCESMetricECSCPU   = "cpu_util"
	huaweiCESMetricECSMem   = "mem_util"
	metricsPerEcsInstance   = 2
)

type ecsUtilWindowAgg struct {
	cpuPeak, cpuAvg, cpuMin float64
	cpuOK                   bool
	memPeak, memAvg, memMin float64
	memOK                   bool
}

func utilizationWindowProto(peak, avg, min float64, ok bool) *pbutilization.UtilizationWindow {
	if !ok {
		return &pbutilization.UtilizationWindow{Available: false}
	}
	return &pbutilization.UtilizationWindow{
		PeakPercent: peak, AvgPercent: avg, MinPercent: min, Available: true,
	}
}

func fillHuaweiECSUtilization(ctx context.Context, ecsList []*pbecs.EcsInstance, regionName string, tenant tenanter.Tenanter, accountName string) {
	if len(ecsList) == 0 {
		return
	}
	cli, err := huaweices.NewClient(regionName, tenant)
	if err != nil {
		glog.Warningf("Huawei ECS CES client init failed account=%s region=%s err=%v", accountName, regionName, err)
		return
	}
	now := time.Now().UTC()
	toMs := now.UnixMilli()
	from30 := now.Add(-30 * 24 * time.Hour).UnixMilli()
	from180 := now.Add(-180 * 24 * time.Hour).UnixMilli()

	ids := make([]string, 0, len(ecsList))
	for _, e := range ecsList {
		if e == nil || e.InstanceId == "" {
			continue
		}
		ids = append(ids, e.InstanceId)
	}
	if len(ids) == 0 {
		return
	}

	m30 := make(map[string]ecsUtilWindowAgg, len(ids))
	m180 := make(map[string]ecsUtilWindowAgg, len(ids))

	for _, batch := range huaweices.ChunkInstanceIDs(ids, metricsPerEcsInstance, huaweices.MaxMetricsPerBatch) {
		q := make([]huaweices.MetricQuery, 0, len(batch)*2)
		for _, id := range batch {
			q = append(q,
				huaweices.MetricQuery{
					Namespace: huaweiCESNamespaceECS, DimName: huaweiCESDimECSInstance,
					DimValue: id, MetricName: huaweiCESMetricECSCPU,
				},
				huaweices.MetricQuery{
					Namespace: huaweiCESNamespaceECS, DimName: huaweiCESDimECSInstance,
					DimValue: id, MetricName: huaweiCESMetricECSMem,
				},
			)
		}
		if series30, err30 := huaweices.BatchQueryAverageSeries(ctx, cli, q, from30, toMs); err30 != nil {
			huaweices.LogBatchError("ECS", accountName, regionName, err30)
		} else {
			for _, id := range batch {
				pc, ac, mc, okc := huaweices.PeakAvgMinFromAveragePoints(series30[id+"\x00"+huaweiCESMetricECSCPU])
				pm, am, mm, okm := huaweices.PeakAvgMinFromAveragePoints(series30[id+"\x00"+huaweiCESMetricECSMem])
				m30[id] = ecsUtilWindowAgg{
					cpuPeak: pc, cpuAvg: ac, cpuMin: mc, cpuOK: okc,
					memPeak: pm, memAvg: am, memMin: mm, memOK: okm,
				}
			}
		}
		if series180, err180 := huaweices.BatchQueryAverageSeries(ctx, cli, q, from180, toMs); err180 != nil {
			huaweices.LogBatchError("ECS", accountName, regionName, err180)
		} else {
			for _, id := range batch {
				pc, ac, mc, okc := huaweices.PeakAvgMinFromAveragePoints(series180[id+"\x00"+huaweiCESMetricECSCPU])
				pm, am, mm, okm := huaweices.PeakAvgMinFromAveragePoints(series180[id+"\x00"+huaweiCESMetricECSMem])
				m180[id] = ecsUtilWindowAgg{
					cpuPeak: pc, cpuAvg: ac, cpuMin: mc, cpuOK: okc,
					memPeak: pm, memAvg: am, memMin: mm, memOK: okm,
				}
			}
		}
	}

	for _, e := range ecsList {
		if e == nil || e.InstanceId == "" {
			continue
		}
		a30 := m30[e.InstanceId]
		a180 := m180[e.InstanceId]
		e.UtilizationAudit = &pbutilization.ComputeUtilizationAudit{
			CpuLast_30D:   utilizationWindowProto(a30.cpuPeak, a30.cpuAvg, a30.cpuMin, a30.cpuOK),
			CpuLast_180D:  utilizationWindowProto(a180.cpuPeak, a180.cpuAvg, a180.cpuMin, a180.cpuOK),
			MemLast_30D:   utilizationWindowProto(a30.memPeak, a30.memAvg, a30.memMin, a30.memOK),
			MemLast_180D:  utilizationWindowProto(a180.memPeak, a180.memAvg, a180.memMin, a180.memOK),
		}
	}
}

func (ecs *HuaweiEcs) ListDetail(ctx context.Context, req *pbecs.ListDetailReq) (*pbecs.ListDetailResp, error) {
	request := new(model.ListServersDetailsRequest)
	offset := (req.PageNumber - 1) * req.PageSize
	request.Offset = &offset
	limit := req.PageSize
	request.Limit = &limit

	resp, err := ecs.cli.ListServersDetails(request)
	if err != nil {
		return nil, errors.Wrap(err, "Huawei ListDetail error")
	}

	servers := *resp.Servers
	n := len(servers)
	glog.Infof("Huawei ECS ListDetail block_device pull begin account=%s region=%s servers_in_page=%d page_number=%d",
		ecs.tenanter.AccountName(), ecs.region.GetName(), n, req.PageNumber)

	type diskRow struct {
		sys, data int32
		summary   string
	}
	disks := make([]diskRow, n)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)
	var diskOK, diskFail int32
	for i := 0; i < n; i++ {
		i := i
		v := servers[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			blkResp, blkErr := ecs.cli.ListServerBlockDevices(&model.ListServerBlockDevicesRequest{ServerId: v.Id})
			if blkErr != nil {
				atomic.AddInt32(&diskFail, 1)
				glog.Warningf("Huawei ListServerBlockDevices failed server_id=%s server_name=%q account=%s region=%s err=%v",
					v.Id, v.Name, ecs.tenanter.AccountName(), ecs.region.GetName(), blkErr)
				disks[i].sys, disks[i].data, disks[i].summary = huaweiDiskFromListServerBlockDevices(nil, v.Flavor)
				glog.Infof("Huawei disk fallback after API error server_id=%s flavor_disk_only sys_gb=%d data_gb=%d summary=%q",
					v.Id, disks[i].sys, disks[i].data, disks[i].summary)
				return
			}
			atomic.AddInt32(&diskOK, 1)
			disks[i].sys, disks[i].data, disks[i].summary = huaweiDiskFromListServerBlockDevices(blkResp, v.Flavor)
			glog.V(2).Infof("Huawei disk from ListServerBlockDevices server_id=%s server_name=%q sys_gb=%d data_gb=%d summary=%q",
				v.Id, v.Name, disks[i].sys, disks[i].data, disks[i].summary)
		}()
	}
	wg.Wait()
	glog.Infof("Huawei ECS ListDetail block_device pull end account=%s region=%s ok=%d fail=%d (use -v=2 for per-instance disk lines)",
		ecs.tenanter.AccountName(), ecs.region.GetName(), diskOK, diskFail)

	ecses := make([]*pbecs.EcsInstance, n)
	for k, v := range servers {
		desc := ""
		if v.Description != nil {
			desc = *v.Description
		}
		pub, priv := huaweiCollectIPs(v.Addresses, v.AccessIPv4)
		resourceGroup := ""
		if v.EnterpriseProjectId != nil {
			resourceGroup = *v.EnterpriseProjectId
		}

		ecses[k] = &pbecs.EcsInstance{
			Provider:         pbtenant.CloudProvider_huawei,
			AccountName:      ecs.tenanter.AccountName(),
			InstanceId:       v.Id,
			InstanceName:     v.Name,
			RegionName:       ecs.region.GetName(),
			InstanceType:     huaweiFlavorName(v.Flavor),
			PublicIps:        pub,
			Cpu:              huaweiFlavorVCPU(v.Flavor),
			Memory:           huaweiFlavorRAMMB(v.Flavor),
			Description:      desc,
			Status:           v.Status,
			CreationTime:     v.Created,
			ExpireTime:       v.OSSRVUSGterminatedAt,
			InnerIps:         priv,
			VpcId:            huaweiMetadataVPC(v.Metadata),
			ResourceGroupId:  resourceGroup,
			ChargeType:       huaweiMetadataChargeType(v.Metadata),
			ImageId:          huaweiECSImageID(&v),
			ImageName:        huaweiMetadataGet(v.Metadata, "image_name"),
			OsType:           huaweiMetadataGet(v.Metadata, "os_type"),
			OsBit:            huaweiMetadataGet(v.Metadata, "os_bit"),
			SystemDiskSizeGb: disks[k].sys,
			DataDiskTotalGb:  disks[k].data,
			DiskSummary:      disks[k].summary,
			EnvTagValue:      envtags.HuaweiECSFromServerDetail(v, envtags.ECSKey()),
			NodeTagValue:     envtags.HuaweiECSFromServerDetail(v, envtags.NodeTagKey()),
		}
	}

	isFinished := false
	if len(ecses) < int(req.PageSize) {
		isFinished = true
	}

	fillHuaweiECSUtilization(ctx, ecses, ecs.region.GetName(), ecs.tenanter, ecs.tenanter.AccountName())

	return &pbecs.ListDetailResp{
		Ecses:      ecses,
		Finished:   isFinished,
		NextToken:  "",
		PageNumber: req.PageNumber + 1,
		PageSize:   req.PageSize,
		RequestId:  "",
	}, nil
}
