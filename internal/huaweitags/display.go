// Package huaweitags 将华为云各服务标签 API 的返回格式化为列表展示字符串（key=value; …），
// 并与多来源标签合并。
//
// 与 ECS 对齐的推荐模式：先列资源（列表/详情里可能带不完整 tags），再按资源 ID 调官方「查询标签」
// 专用接口，合并时用 MergePairsPreferPrimary(专用接口返回, 列表内联)，即专用接口优先。
// 各产品对应华为文档中的查询标签接口（SDK 方法名）：
//
//	ECS   ShowServerTags
//	RDS   ListInstanceTags
//	DCS   ShowTags
//	DMS Kafka  ShowKafkaTags
//	EIP   ShowPublicipTags
//	ELB   ShowLoadbalancerTags（OpenAPI v2；列表侧可用 ELB v3 ListLoadBalancers.tags 作补全）
//	CCE   GetResourceTags（resource_type=cce-cluster，resource_id=集群 metadata.uid）
package huaweitags

import (
	"strings"

	ccemodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cce/v3/model"
	dcsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/dcs/v2/model"
	elbv2model "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v2/model"
	elbmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3/model"
	ecsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	eipmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/eip/v2/model"
	kafkamodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/kafka/v2/model"
	rdsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3/model"
)

// FormatPairsDisplay 将 (key,value) 列表格式化为可读字符串；无标签时返回空串。
func FormatPairsDisplay(pairs [][2]string) string {
	if len(pairs) == 0 {
		return ""
	}
	const maxPairs = 80
	var b strings.Builder
	n := 0
	for _, p := range pairs {
		if len(p) < 2 {
			continue
		}
		k := strings.TrimSpace(p[0])
		if k == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(strings.TrimSpace(p[1]))
		n++
		if n >= maxPairs {
			b.WriteString("; …")
			break
		}
	}
	return b.String()
}

// MergeRDSUserDisplayPairs 用于「用户标签(展示)」：以 ListInstanceTags 中 tag_type 为用户的主结果，
// 用列表接口 tags 补键；排除在 API 中已标为 system 的键，避免用户列混入华为系统类标签。
func MergeRDSUserDisplayPairs(sysPairs, usrPairs, listPairs [][2]string) [][2]string {
	sysKeys := make(map[string]struct{}, len(sysPairs))
	for _, p := range sysPairs {
		if len(p) < 2 {
			continue
		}
		sysKeys[strings.ToLower(strings.TrimSpace(p[0]))] = struct{}{}
	}
	var listFiltered [][2]string
	for _, p := range listPairs {
		if len(p) < 2 {
			continue
		}
		if _, ok := sysKeys[strings.ToLower(strings.TrimSpace(p[0]))]; ok {
			continue
		}
		listFiltered = append(listFiltered, p)
	}
	return MergePairsPreferPrimary(usrPairs, listFiltered)
}

// FilterPairsExcludingHuaweiSysPrefix 去掉键名以 _sys_ 开头的标签（华为常见系统类键），用于 DCS/Kafka 等无 tag_type 区分的展示列。
func FilterPairsExcludingHuaweiSysPrefix(pairs [][2]string) [][2]string {
	if len(pairs) == 0 {
		return nil
	}
	var out [][2]string
	for _, p := range pairs {
		if len(p) < 2 {
			continue
		}
		k := strings.TrimSpace(p[0])
		if strings.HasPrefix(strings.ToLower(k), "_sys_") {
			continue
		}
		out = append(out, p)
	}
	return out
}

// MergePairsPreferPrimary 合并两组标签：primary 中已有的键不再被 secondary 覆盖；secondary 仅补充缺失键。
func MergePairsPreferPrimary(primary, secondary [][2]string) [][2]string {
	seen := make(map[string]struct{}, len(primary)+len(secondary))
	var out [][2]string
	add := func(pairs [][2]string) {
		for _, p := range pairs {
			if len(p) < 2 {
				continue
			}
			k := strings.TrimSpace(strings.ToLower(p[0]))
			if k == "" {
				continue
			}
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, [2]string{strings.TrimSpace(p[0]), strings.TrimSpace(p[1])})
		}
	}
	add(primary)
	add(secondary)
	return out
}

// FormatECSSystemTagsDisplay 由 ListServersDetails 的 sys_tags 生成「系统标签」展示串。
func FormatECSSystemTagsDisplay(sysTags *[]ecsmodel.ServerSystemTag) string {
	if sysTags == nil {
		return ""
	}
	var pairs [][2]string
	for _, st := range *sysTags {
		if st.Key == nil {
			continue
		}
		k := strings.TrimSpace(*st.Key)
		if k == "" {
			continue
		}
		v := ""
		if st.Value != nil {
			v = strings.TrimSpace(*st.Value)
		}
		pairs = append(pairs, [2]string{k, v})
	}
	return FormatPairsDisplay(pairs)
}

// SplitRDSInstanceTags 按 RDS「查询实例标签」返回的 tag_type 区分系统标签与用户标签。
// tag_type 为 system（大小写不敏感）归入系统；其余归入用户自定义。
func SplitRDSInstanceTags(tags *[]rdsmodel.ResourceTag) (systemPairs, userPairs [][2]string) {
	if tags == nil {
		return nil, nil
	}
	for _, tg := range *tags {
		k := strings.TrimSpace(tg.Key)
		if k == "" {
			continue
		}
		v := strings.TrimSpace(tg.Value)
		p := [2]string{k, v}
		if strings.EqualFold(strings.TrimSpace(tg.TagType), "system") {
			systemPairs = append(systemPairs, p)
		} else {
			userPairs = append(userPairs, p)
		}
	}
	return systemPairs, userPairs
}

// PairsFromRDSListInstanceTags 将 ListInstanceTags 全量转为合并用键值列表（含系统与用户，用于合并兜底）。
func PairsFromRDSListInstanceTags(tags *[]rdsmodel.ResourceTag) [][2]string {
	if tags == nil {
		return nil
	}
	sys, usr := SplitRDSInstanceTags(tags)
	return append(sys, usr...)
}

// PairsFromEIPShowPublicipTags 解析 EIP ShowPublicipTags 响应。
func PairsFromEIPShowPublicipTags(tags *[]eipmodel.ResourceTagResp) [][2]string {
	if tags == nil {
		return nil
	}
	var out [][2]string
	for _, tg := range *tags {
		if tg.Key == nil {
			continue
		}
		k := strings.TrimSpace(*tg.Key)
		if k == "" {
			continue
		}
		v := ""
		if tg.Value != nil {
			v = strings.TrimSpace(*tg.Value)
		}
		out = append(out, [2]string{k, v})
	}
	return out
}

// PairsFromDCSResourceTags 解析 DCS ShowTags / 列表中的 ResourceTag。
func PairsFromDCSResourceTags(tags *[]dcsmodel.ResourceTag) [][2]string {
	if tags == nil {
		return nil
	}
	var out [][2]string
	for _, tg := range *tags {
		k := strings.TrimSpace(tg.Key)
		if k == "" {
			continue
		}
		v := ""
		if tg.Value != nil {
			v = strings.TrimSpace(*tg.Value)
		}
		out = append(out, [2]string{k, v})
	}
	return out
}

// PairsFromKafkaTagEntities 解析 Kafka ShowKafkaTags 的 TagEntity 列表。
func PairsFromKafkaTagEntities(tags *[]kafkamodel.TagEntity) [][2]string {
	if tags == nil {
		return nil
	}
	var out [][2]string
	for _, tg := range *tags {
		if tg.Key == nil {
			continue
		}
		k := strings.TrimSpace(*tg.Key)
		if k == "" {
			continue
		}
		v := ""
		if tg.Value != nil {
			v = strings.TrimSpace(*tg.Value)
		}
		out = append(out, [2]string{k, v})
	}
	return out
}

// PairsFromELBShowLoadbalancerTags 解析 ELB v2「查询负载均衡器标签」ShowLoadbalancerTags 响应。
func PairsFromELBShowLoadbalancerTags(tags *[]elbv2model.ResourceTag) [][2]string {
	if tags == nil {
		return nil
	}
	var out [][2]string
	for _, tg := range *tags {
		k := strings.TrimSpace(tg.Key)
		if k == "" {
			continue
		}
		out = append(out, [2]string{k, strings.TrimSpace(tg.Value)})
	}
	return out
}

// PairsFromCCEResourceTags 解析 CCE GetResourceTags 返回的自定义标签列表（tags 字段）。
func PairsFromCCEResourceTags(tags *[]ccemodel.ResourceTag) [][2]string {
	if tags == nil {
		return nil
	}
	var out [][2]string
	for _, tg := range *tags {
		if tg.Key == nil {
			continue
		}
		k := strings.TrimSpace(*tg.Key)
		if k == "" {
			continue
		}
		v := ""
		if tg.Value != nil {
			v = strings.TrimSpace(*tg.Value)
		}
		out = append(out, [2]string{k, v})
	}
	return out
}

// PairsFromStringMap 将 map 转为合并用 (key,value) 列表（键名保留原样）。
func PairsFromStringMap(m map[string]string) [][2]string {
	if len(m) == 0 {
		return nil
	}
	out := make([][2]string, 0, len(m))
	for k, v := range m {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out = append(out, [2]string{k, strings.TrimSpace(v)})
	}
	return out
}

// PairsFromELBTags 解析 ELB ListLoadBalancers 返回的负载均衡标签。
func PairsFromELBTags(tags []elbmodel.Tag) [][2]string {
	var out [][2]string
	for _, tg := range tags {
		if tg.Key == nil {
			continue
		}
		k := strings.TrimSpace(*tg.Key)
		if k == "" {
			continue
		}
		v := ""
		if tg.Value != nil {
			v = strings.TrimSpace(*tg.Value)
		}
		out = append(out, [2]string{k, v})
	}
	return out
}
