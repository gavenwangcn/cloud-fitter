package cmdb

import "strings"

// isEmptyCloudSyncValue 判断云平台同步字段是否视为「空」（空则不覆盖 CMDB 已有非空值）。
func isEmptyCloudSyncValue(v any) bool {
	if v == nil {
		return true
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) == ""
	case []string:
		if len(t) == 0 {
			return true
		}
		for _, s := range t {
			if strings.TrimSpace(s) != "" {
				return false
			}
		}
		return true
	case []any:
		if len(t) == 0 {
			return true
		}
		for _, x := range t {
			if !isEmptyCloudSyncValue(x) {
				return false
			}
		}
		return true
	default:
		return strings.TrimSpace(anyToCompareStr(v)) == ""
	}
}

func cmExistingHasValue(v any) bool {
	return !isEmptyCloudSyncValue(v)
}

// mergeCMDBPreserveNonEmpty 更新 CI 前合并：云侧字段为空且 CMDB 已有值时保留 CMDB 原值。
func mergeCMDBPreserveNonEmpty(want, existing map[string]any) map[string]any {
	if want == nil {
		return nil
	}
	if existing == nil {
		out := make(map[string]any, len(want))
		for k, v := range want {
			out[k] = v
		}
		return out
	}
	out := make(map[string]any, len(want))
	for k, wv := range want {
		if isEmptyCloudSyncValue(wv) && cmExistingHasValue(existing[k]) {
			out[k] = existing[k]
			continue
		}
		out[k] = wv
	}
	return out
}

// cloudDomainListEmpty 云侧域名列表为空（含 WAF 未匹配）。
func cloudDomainListEmpty(domains []string) bool {
	return len(domainNamesForCMDB(domains)) == 0
}
