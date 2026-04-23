package envtags

import "strings"

// FromPairs 按 (key,value) 列表查找，键名大小写不敏感。
func FromPairs(tagKey string, pairs [][2]string) string {
	if tagKey == "" {
		return ""
	}
	want := strings.TrimSpace(tagKey)
	for _, p := range pairs {
		if len(p) < 2 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(p[0]), want) {
			return strings.TrimSpace(p[1])
		}
	}
	return ""
}

// FromMap 通用 map（键大小写不敏感比较）。
func FromMap(tagKey string, m map[string]string) string {
	if tagKey == "" || len(m) == 0 {
		return ""
	}
	want := strings.TrimSpace(tagKey)
	for k, v := range m {
		if strings.EqualFold(strings.TrimSpace(k), want) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
