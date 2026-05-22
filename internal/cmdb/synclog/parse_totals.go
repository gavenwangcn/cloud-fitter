package synclog

import (
	"encoding/json"
	"strings"
)

// parseResourceTotalsJSON 解析 resource_fail_totals_json，兼容旧版纯计数格式 {"ECS":3}。
func parseResourceTotalsJSON(raw string) map[string]ResourceFailTotal {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return map[string]ResourceFailTotal{}
	}
	var detailed map[string]ResourceFailTotal
	if err := json.Unmarshal([]byte(raw), &detailed); err == nil && len(detailed) > 0 {
		// 新格式：值为对象；旧格式 unmarshal 后 FailCount 均为 0，需再试 legacy
		for _, v := range detailed {
			if v.FailCount > 0 || len(v.Items) > 0 {
				return detailed
			}
		}
	}
	var legacy map[string]int
	if err := json.Unmarshal([]byte(raw), &legacy); err == nil {
		out := make(map[string]ResourceFailTotal, len(legacy))
		for k, v := range legacy {
			if v <= 0 {
				continue
			}
			out[k] = ResourceFailTotal{FailCount: v}
		}
		return out
	}
	if detailed != nil {
		return detailed
	}
	return map[string]ResourceFailTotal{}
}
