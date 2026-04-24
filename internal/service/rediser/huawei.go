package rediser

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hwdcs "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/dcs/v2"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/dcs/v2/model"
	hwiam "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3"
	iammodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3/model"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbredis"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/envtags"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweicloudregion"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

type HuaweiDcs struct {
	cli      *hwdcs.DcsClient
	region   tenanter.Region
	tenanter tenanter.Tenanter
}

func newHuaweiDcsClient(region tenanter.Region, tenant tenanter.Tenanter) (Rediser, error) {
	var (
		client *hwdcs.DcsClient
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
		hcClient := hwdcs.DcsClientBuilder().WithRegion(huaweicloudregion.EndpointForService("dcs", rName)).WithCredential(auth).Build()
		client = hwdcs.NewDcsClient(hcClient)
	default:
	}

	if err != nil {
		return nil, errors.Wrap(err, "init huawei redis client error")
	}
	return &HuaweiDcs{
		cli:      client,
		region:   region,
		tenanter: tenant,
	}, nil
}

func dcsChargingMode(m *int32) string {
	if m == nil {
		return ""
	}
	switch *m {
	case 0:
		return "postPaid"
	case 1:
		return "prePaid"
	default:
		return strconv.FormatInt(int64(*m), 10)
	}
}

func splitDcsIPs(s *string) []string {
	if s == nil {
		return nil
	}
	raw := strings.TrimSpace(*s)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == ';'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// 华为规格名中常见片段如 2u4g、8U16G 等
var dcsSpecCPURe = regexp.MustCompile(`(?i)(\d+)u`)

func cpuFromAttrs(attrs *[]model.AttrsObject) int32 {
	if attrs == nil {
		return 0
	}
	for _, a := range *attrs {
		if a.Name == nil || a.Value == nil {
			continue
		}
		n := strings.ToLower(strings.TrimSpace(*a.Name))
		if strings.Contains(n, "cpu") || n == "vcpu" {
			v, err := strconv.ParseInt(strings.TrimSpace(*a.Value), 10, 32)
			if err == nil && v > 0 {
				return int32(v)
			}
		}
	}
	return 0
}

func cpuFromSpecCodeFallback(spec string) int32 {
	if spec == "" {
		return 0
	}
	m := dcsSpecCPURe.FindStringSubmatch(spec)
	if len(m) < 2 {
		return 0
	}
	v, err := strconv.ParseInt(m[1], 10, 32)
	if err != nil {
		return 0
	}
	return int32(v)
}

func flavorItemCPU(item *model.FlavorsItems) int32 {
	if item == nil {
		return 0
	}
	if c := cpuFromAttrs(item.Attrs); c > 0 {
		return c
	}
	return cpuFromSpecCodeFallback(derefString(item.SpecCode))
}

// buildFlavorCPUBySpec 调用 ListFlavors，将 spec_code -> vCPU；失败时返回空 map，由实例 spec 再尝试正则兜底。
func (redis *HuaweiDcs) buildFlavorCPUBySpec() map[string]int32 {
	out := make(map[string]int32)
	req := new(model.ListFlavorsRequest)
	resp, err := redis.cli.ListFlavors(req)
	if err != nil || resp == nil || resp.Flavors == nil {
		return out
	}
	for i := range *resp.Flavors {
		item := &(*resp.Flavors)[i]
		spec := strings.TrimSpace(derefString(item.SpecCode))
		if spec == "" {
			continue
		}
		cpu := flavorItemCPU(item)
		if cpu > out[spec] {
			out[spec] = cpu
		}
	}
	return out
}

func resolveDcsCPU(spec string, bySpec map[string]int32) int32 {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0
	}
	if bySpec != nil {
		if c := bySpec[spec]; c > 0 {
			return c
		}
	}
	return cpuFromSpecCodeFallback(spec)
}

func (redis *HuaweiDcs) ListDetail(ctx context.Context, req *pbredis.ListDetailReq) (*pbredis.ListDetailResp, error) {
	request := new(model.ListInstancesRequest)
	offset := (req.PageNumber - 1) * req.PageSize
	request.Offset = &offset
	limit := req.PageSize
	request.Limit = &limit

	resp, err := redis.cli.ListInstances(request)
	if err != nil {
		return nil, errors.Wrap(err, "Huawei ListDetail error")
	}

	instances := *resp.Instances
	cpuBySpec := redis.buildFlavorCPUBySpec()

	redises := make([]*pbredis.RedisInstance, len(instances))
	for k, v := range instances {
		size := int32(0)
		if v.MaxMemory != nil {
			size = *v.MaxMemory
		}
		used := int32(0)
		if v.UsedMemory != nil {
			used = *v.UsedMemory
		}
		var pub []string
		if v.PublicipAddress != nil && strings.TrimSpace(*v.PublicipAddress) != "" {
			pub = []string{strings.TrimSpace(*v.PublicipAddress)}
		}
		priv := splitDcsIPs(v.Ip)
		spec := derefString(v.SpecCode)

		var tagPairs [][2]string
		if v.Tags != nil {
			for _, tg := range *v.Tags {
				val := ""
				if tg.Value != nil {
					val = *tg.Value
				}
				tagPairs = append(tagPairs, [2]string{tg.Key, val})
			}
		}

		redises[k] = &pbredis.RedisInstance{
			Provider:       pbtenant.CloudProvider_huawei,
			AccoutName:     redis.tenanter.AccountName(),
			InstanceId:     derefString(v.InstanceId),
			InstanceName:   derefString(v.Name),
			RegionName:     redis.region.GetName(),
			Size:           size,
			Status:         derefString(v.Status),
			CreationTime:   derefString(v.CreatedAt),
			ExpireTime:     "",
			SpecCode:       spec,
			VpcId:          derefString(v.VpcId),
			PublicIps:      pub,
			PrivateIps:     priv,
			UsedMemoryMb:   used,
			ChargeType:     dcsChargingMode(v.ChargingMode),
			Cpu:            resolveDcsCPU(spec, cpuBySpec),
			EnvTagValue:    envtags.FromPairs(envtags.RedisKey(), tagPairs),
			NodeTagValue:   envtags.FromPairs(envtags.NodeTagKey(), tagPairs),
		}
	}

	isFinished := false
	if len(redises) < int(req.PageSize) {
		isFinished = true
	}

	return &pbredis.ListDetailResp{
		Redises:    redises,
		Finished:   isFinished,
		NextToken:  "",
		PageNumber: req.PageNumber + 1,
		PageSize:   req.PageSize,
		RequestId:  "",
	}, nil
}
