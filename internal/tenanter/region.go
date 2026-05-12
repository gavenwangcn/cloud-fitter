package tenanter

import (
	"strings"

	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
)

var (
	ErrNoExistAliRegionId     = errors.New("region id not exist in ali")
	ErrNoExistTencentRegionId = errors.New("region id not exist in tencent")
	ErrNoExistHuaweiRegionId  = errors.New("region id not exist in huawei")
	ErrNoExistAwsRegionId     = errors.New("region id not exist in aws")
)

type Region interface {
	GetId() int32
	GetName() string
}

// huaweiTurkeyTrWest1RegionNum 与 idl/pbtenant tenant.proto 中 hw_tr_west_1 枚举号一致（OpenAPI region：tr-west-1）。
const huaweiTurkeyTrWest1RegionNum int32 = 5

type region struct {
	provider   pbtenant.CloudProvider
	regionId   int32
	regionName string
}

func NewRegion(provider pbtenant.CloudProvider, regionId int32) (Region, error) {
	r := &region{
		provider: provider,
		regionId: regionId,
	}
	var err error

	switch provider {
	case pbtenant.CloudProvider_ali:
		r.regionName, err = getAliRegionName(regionId)
	case pbtenant.CloudProvider_tencent:
		r.regionName, err = getTencentRegionName(regionId)
	case pbtenant.CloudProvider_huawei:
		r.regionName, err = getHuaweiRegionName(regionId)
	case pbtenant.CloudProvider_aws:
		r.regionName, err = getAwsRegionName(regionId)
	}

	return r, err
}

func (r *region) GetName() string {
	return r.regionName
}

func (r *region) GetId() int32 {
	return r.regionId
}

// RegionsForProviderAndTenant 返回该云账号下需扫描的地域：华为云按账号区域类型（国内 / 俄罗斯 / 土耳其）分流，其它云与 GetAllRegionIds 一致。
func RegionsForProviderAndTenant(provider pbtenant.CloudProvider, tenant Tenanter) []Region {
	if provider != pbtenant.CloudProvider_huawei {
		return GetAllRegionIds(provider)
	}
	return huaweiRegionsForTenant(tenant)
}

func huaweiRegionsForTenant(tenant Tenanter) []Region {
	ak, ok := tenant.(*AccessKeyTenant)
	if !ok {
		return huaweiDomesticRegionSlice()
	}
	if ak.HuaweiAccountScope() == HuaweiAccountScopeRussia {
		r, err := NewRegion(pbtenant.CloudProvider_huawei, int32(pbtenant.HuaweiRegionId_hw_ru_moscow_1))
		if err != nil {
			return nil
		}
		return []Region{r}
	}
	if ak.HuaweiAccountScope() == HuaweiAccountScopeTurkey {
		r, err := NewRegion(pbtenant.CloudProvider_huawei, huaweiTurkeyTrWest1RegionNum)
		if err != nil {
			return nil
		}
		return []Region{r}
	}
	return huaweiDomesticRegionSlice()
}

// 国内：华东3（cn-east-3）、华东4/上海二（cn-east-4）、香港（ap-southeast-1）。
func huaweiDomesticRegionSlice() []Region {
	ids := []int32{
		int32(pbtenant.HuaweiRegionId_hw_cn_east_3),
		int32(pbtenant.HuaweiRegionId_hw_cn_east_4),
		int32(pbtenant.HuaweiRegionId_hw_ap_southeast_1),
	}
	var out []Region
	for _, id := range ids {
		r, err := NewRegion(pbtenant.CloudProvider_huawei, id)
		if err != nil {
			continue
		}
		out = append(out, r)
	}
	return out
}

func GetAllRegionIds(provider pbtenant.CloudProvider) (regions []Region) {
	switch provider {
	case pbtenant.CloudProvider_ali:
		for rId := range pbtenant.AliRegionId_name {
			if rId != int32(pbtenant.AliRegionId_ali_all) {
				region, _ := NewRegion(provider, rId)
				regions = append(regions, region)
			}
		}
	case pbtenant.CloudProvider_tencent:
		for rId := range pbtenant.TencentRegionId_name {
			if rId != int32(pbtenant.TencentRegionId_tc_all) {
				region, _ := NewRegion(provider, rId)
				regions = append(regions, region)
			}
		}
	case pbtenant.CloudProvider_huawei:
		for rId := range pbtenant.HuaweiRegionId_name {
			if rId != int32(pbtenant.HuaweiRegionId_hw_all) {
				region, _ := NewRegion(provider, rId)
				regions = append(regions, region)
			}
		}
	case pbtenant.CloudProvider_aws:
		for rId := range pbtenant.AwsRegionId_name {
			if rId != int32(pbtenant.AwsRegionId_aws_all) {
				region, _ := NewRegion(provider, rId)
				regions = append(regions, region)
			}
		}
	}

	return
}

// prefix ali_
func getAliRegionName(regionId int32) (string, error) {
	name, ok := pbtenant.AliRegionId_name[regionId]
	if !ok || regionId == int32(pbtenant.AliRegionId_ali_all) {
		return "", errors.WithMessagef(ErrNoExistAliRegionId, "input region id is %d", regionId)
	}
	region := strings.ReplaceAll(name, "_", "-")
	return region[4:], nil
}

// prefix tc_
func getTencentRegionName(regionId int32) (string, error) {
	name, ok := pbtenant.TencentRegionId_name[regionId]
	if !ok || regionId == int32(pbtenant.TencentRegionId_tc_all) {
		return "", errors.WithMessagef(ErrNoExistTencentRegionId, "input region id is %d", regionId)
	}
	region := strings.ReplaceAll(name, "_", "-")
	return region[3:], nil
}

// prefix hw_
func getHuaweiRegionName(regionId int32) (string, error) {
	// 土耳其（伊斯坦布尔）tr-west-1；与 tenant.proto hw_tr_west_1=5 一致（见华为云终端节点文档）。
	if regionId == huaweiTurkeyTrWest1RegionNum {
		return "tr-west-1", nil
	}
	name, ok := pbtenant.HuaweiRegionId_name[regionId]
	if !ok || regionId == int32(pbtenant.HuaweiRegionId_hw_all) {
		return "", errors.WithMessagef(ErrNoExistHuaweiRegionId, "input region id is %d", regionId)
	}
	region := strings.ReplaceAll(name, "_", "-")
	return region[3:], nil
}

// prefix aws_
func getAwsRegionName(regionId int32) (string, error) {
	name, ok := pbtenant.AwsRegionId_name[regionId]
	if !ok || regionId == int32(pbtenant.AwsRegionId_aws_all) {
		return "", errors.WithMessagef(ErrNoExistAwsRegionId, "input region id is %d", regionId)
	}
	region := strings.ReplaceAll(name, "_", "-")
	return region[4:], nil
}
