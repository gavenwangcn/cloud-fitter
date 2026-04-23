package ecser

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hwecs "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	hwregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/region"
	hwevs "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/evs/v2"
	evsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/evs/v2/model"
	evsregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/evs/v2/region"
	hwiam "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3"
	iammodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3/model"
	iamregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3/region"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbecs"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

type HuaweiEcs struct {
	cli      *hwecs.EcsClient
	evs      *hwevs.EvsClient
	region   tenanter.Region
	tenanter tenanter.Tenanter
}

func newHuaweiEcsClient(region tenanter.Region, tenant tenanter.Tenanter) (Ecser, error) {
	var (
		client    *hwecs.EcsClient
		evsClient *hwevs.EvsClient
		err       error
	)

	switch t := tenant.(type) {
	case *tenanter.AccessKeyTenant:
		auth := basic.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).Build()
		rName := region.GetName()
		cli := hwiam.IamClientBuilder().WithRegion(iamregion.ValueOf(rName)).WithCredential(auth).Build()
		c := hwiam.NewIamClient(cli)
		request := new(iammodel.KeystoneListProjectsRequest)
		request.Name = &rName
		r, err := c.KeystoneListProjects(request)
		if err != nil || len(*r.Projects) == 0 {
			return nil, errors.Wrapf(err, "Huawei KeystoneListProjects regionName %s", rName)
		}
		projectId := (*r.Projects)[0].Id

		auth = basic.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).WithProjectId(projectId).Build()
		hcEcs := hwecs.EcsClientBuilder().WithRegion(hwregion.ValueOf(rName)).WithCredential(auth).Build()
		client = hwecs.NewEcsClient(hcEcs)
		hcEvs := hwevs.EvsClientBuilder().WithRegion(evsregion.ValueOf(rName)).WithCredential(auth).Build()
		evsClient = hwevs.NewEvsClient(hcEvs)
	default:
	}

	if err != nil {
		return nil, errors.Wrap(err, "init huawei ecs client error")
	}
	return &HuaweiEcs{
		cli:      client,
		evs:      evsClient,
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

func huaweiUniqNonEmpty(ids []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func huaweiBootIndexIsZero(bi *string) bool {
	if bi == nil {
		return false
	}
	s := strings.TrimSpace(*bi)
	return s == "0"
}

func huaweiIsBootVolume(att model.ServerExtendVolumeAttachment, vol *evsmodel.VolumeDetail) bool {
	if vol != nil && strings.EqualFold(vol.Bootable, "true") {
		return true
	}
	return huaweiBootIndexIsZero(att.BootIndex)
}

// huaweiDiskInfo 结合 ECS 挂载信息（volume id）与 EVS 云硬盘详情（size、bootable）。
// 官方说明：ECS flavor.disk 常为 0 或无效；容量需查 EVS「查询所有云硬盘详情」接口。
func huaweiDiskInfo(v *model.ServerDetail, volByID map[string]evsmodel.VolumeDetail) (sysGB, dataGB int32, summary string) {
	var parts []string
	var sys int32
	var dataSum int32
	for _, att := range v.OsExtendedVolumesvolumesAttached {
		if att.Id == "" {
			continue
		}
		vol, ok := volByID[att.Id]
		if !ok {
			continue
		}
		if huaweiIsBootVolume(att, &vol) {
			sys = vol.Size
			parts = append(parts, fmt.Sprintf("系统盘:%dGB(%s)", vol.Size, vol.VolumeType))
		} else {
			dataSum += vol.Size
			parts = append(parts, fmt.Sprintf("数据盘:%dGB(%s)", vol.Size, vol.VolumeType))
		}
	}
	// flavor.disk 仅作兜底（文档称多数场景无效）
	if sys == 0 && v.Flavor != nil && v.Flavor.Disk != "" {
		if n, err := strconv.ParseInt(v.Flavor.Disk, 10, 32); err == nil && n > 0 {
			sys = int32(n)
		}
	}
	return sys, dataSum, strings.Join(parts, "; ")
}

func (ecs *HuaweiEcs) huaweiFetchVolumesByIDs(ids []string) map[string]evsmodel.VolumeDetail {
	out := make(map[string]evsmodel.VolumeDetail)
	if ecs.evs == nil || len(ids) == 0 {
		return out
	}
	uniq := huaweiUniqNonEmpty(ids)
	const batch = 40
	lim := int32(1000)
	for i := 0; i < len(uniq); i += batch {
		end := i + batch
		if end > len(uniq) {
			end = len(uniq)
		}
		idParam := strings.Join(uniq[i:end], ",")
		req := &evsmodel.ListVolumesRequest{
			Ids:   &idParam,
			Limit: &lim,
		}
		resp, err := ecs.evs.ListVolumes(req)
		if err != nil || resp.Volumes == nil {
			continue
		}
		for _, vol := range *resp.Volumes {
			out[vol.Id] = vol
		}
	}
	return out
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
	var volIDs []string
	for _, v := range servers {
		for _, a := range v.OsExtendedVolumesvolumesAttached {
			if a.Id != "" {
				volIDs = append(volIDs, a.Id)
			}
		}
	}
	volByID := ecs.huaweiFetchVolumesByIDs(volIDs)

	ecses := make([]*pbecs.EcsInstance, len(servers))
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
		sysGB, dataGB, dsum := huaweiDiskInfo(&v, volByID)
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
			SystemDiskSizeGb: sysGB,
			DataDiskTotalGb:  dataGB,
			DiskSummary:      dsum,
		}
	}

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
		RequestId:  "",
	}, nil
}
