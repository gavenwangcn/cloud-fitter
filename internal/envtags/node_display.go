package envtags

import (
	"strings"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
)

// CloudTypeLabelZH 与 CMDB / 列表「云类型」对应的中文云名。
func CloudTypeLabelZH(p pbtenant.CloudProvider) string {
	switch p {
	case pbtenant.CloudProvider_huawei:
		return "华为云"
	case pbtenant.CloudProvider_ali:
		return "阿里云"
	case pbtenant.CloudProvider_tencent:
		return "腾讯云"
	case pbtenant.CloudProvider_aws:
		return "AWS"
	default:
		return "云"
	}
}

// FormatNodeTagDisplay 列表「节点(标签)」展示：「云中文名-地域」；若有节点语义（标签或名字推断）再拼接「-语义」。
func FormatNodeTagDisplay(cloudZH, region, semantic string) string {
	cloudZH = strings.TrimSpace(cloudZH)
	region = strings.TrimSpace(region)
	semantic = strings.TrimSpace(semantic)
	var base string
	if cloudZH != "" && region != "" {
		base = cloudZH + "-" + region
	} else if region != "" {
		base = region
	} else if cloudZH != "" {
		base = cloudZH
	}
	if semantic == "" {
		return base
	}
	if base == "" {
		return semantic
	}
	return base + "-" + semantic
}
