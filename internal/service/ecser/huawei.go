package ecser

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hwecs "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	hwregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/region"
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
		hcClient := hwecs.EcsClientBuilder().WithRegion(hwregion.ValueOf(rName)).WithCredential(auth).Build()
		client = hwecs.NewEcsClient(hcClient)
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
		ecses[k] = &pbecs.EcsInstance{
			Provider:        pbtenant.CloudProvider_huawei,
			AccountName:     ecs.tenanter.AccountName(),
			InstanceId:      v.Id,
			InstanceName:    v.Name,
			RegionName:      ecs.region.GetName(),
			InstanceType:    huaweiFlavorName(v.Flavor),
			PublicIps:       pub,
			Cpu:             huaweiFlavorVCPU(v.Flavor),
			Memory:          huaweiFlavorRAMMB(v.Flavor),
			Description:     desc,
			Status:          v.Status,
			CreationTime:    v.Created,
			ExpireTime:      v.OSSRVUSGterminatedAt,
			InnerIps:        priv,
			VpcId:           huaweiMetadataVPC(v.Metadata),
			ResourceGroupId: resourceGroup,
			ChargeType:      huaweiMetadataChargeType(v.Metadata),
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
