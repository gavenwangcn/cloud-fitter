// Package huaweitags 将华为云各服务标签 API 的返回格式化为列表展示字符串（key=value; …），
// 并与多来源标签合并，便于与官方「查询实例/资源标签」类接口配合使用。
package huaweitags

import (
	"strings"

	dcsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/dcs/v2/model"
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
