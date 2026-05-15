package envtags

import "strings"

// EnvTagOrNameFallback 在环境标签值为空（无键或值为空）时，按资源名推断。
// 名中属性多以 -、_ 分段（如 DFT工时系统-PROD-应用-0002）；在每段内匹配英文
// PROD、TEST、DEV、UAT 或中文 生产、测试、开发、验收（大小写不敏感仅作用于英文段）。
// 多个命中时：先按段从左到右，再按段内出现位置取最先命中者。
func EnvTagOrNameFallback(tagValue, resourceName string) string {
	if s := strings.TrimSpace(tagValue); s != "" {
		return s
	}
	return inferEnvFromName(resourceName)
}

// NodeTagOrNameFallback 在节点标签值为空时，按资源名推断。
// 同样先按 -、_ 分段后在各段内匹配：若全名中同时出现 DFT 与 LC（段内子串、英文不区分大小写），
// 则节点语义为「（德非图&宇海）」，且优先于单侧 DFT/LC 的先后规则及整段 ALL→全部。
// 否则：整段为 ALL（不区分大小写）→全部；DFT→德非图，LC→宇海（后两者英文不区分大小写）；
// 多个命中时：先按段从左到右，再按段内位置取最先命中者。
// 若标签值本身为 ALL，亦表示全部（与名字规则一致）。
func NodeTagOrNameFallback(tagValue, resourceName string) string {
	if s := strings.TrimSpace(tagValue); s != "" {
		if strings.EqualFold(s, "ALL") {
			return "全部"
		}
		return s
	}
	return inferNodeFromName(resourceName)
}

// ResourceNameForTagFallback 用于名字规则：优先实例展示名；为空时尝试标签 Name；再为空用 instanceID。
func ResourceNameForTagFallback(instanceName, instanceID string, tagPairs [][2]string) string {
	if s := strings.TrimSpace(instanceName); s != "" {
		return s
	}
	if n := strings.TrimSpace(FromPairs("Name", tagPairs)); n != "" {
		return n
	}
	return strings.TrimSpace(instanceID)
}

// nameSegments 按 -、_ 切段；若切段后为空（如全为分隔符）则退回整段 trimmed 名。
func nameSegments(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_'
	})
	var out []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return []string{name}
	}
	return out
}

func inferEnvFromName(name string) string {
	segs := nameSegments(name)
	if len(segs) == 0 {
		return ""
	}
	// (segmentIndex, indexInSegment, display)
	type hit struct {
		si, pos int
		val     string
	}
	var best *hit
	for si, seg := range segs {
		upper := strings.ToUpper(seg)
		for _, x := range []struct {
			sub, disp string
			ascii     bool
		}{
			{"生产", "生产", false},
			{"测试", "测试", false},
			{"开发", "开发", false},
			{"验收", "验收", false},
			{"UAT", "验收", true},
			{"PROD", "生产", true},
			{"TEST", "测试", true},
			{"DEV", "开发", true},
		} {
			var idx int
			if x.ascii {
				idx = strings.Index(upper, x.sub)
			} else {
				idx = strings.Index(seg, x.sub)
			}
			if idx < 0 {
				continue
			}
			if best == nil || si < best.si || (si == best.si && idx < best.pos) {
				best = &hit{si, idx, x.disp}
			}
		}
	}
	if best == nil {
		return ""
	}
	return best.val
}

// nameContainsNodeToken 与 inferNodeFromName 中段内匹配方式一致：各段转大写后子串查找。
func nameContainsNodeToken(name, asciiToken string) bool {
	for _, seg := range nameSegments(name) {
		if strings.Contains(strings.ToUpper(strings.TrimSpace(seg)), asciiToken) {
			return true
		}
	}
	return false
}

func inferNodeFromName(name string) string {
	segs := nameSegments(name)
	if len(segs) == 0 {
		return ""
	}
	if nameContainsNodeToken(name, "DFT") && nameContainsNodeToken(name, "LC") {
		return "（德非图&宇海）"
	}
	type hit struct {
		si, pos int
		val     string
	}
	var best *hit
	for si, seg := range segs {
		t := strings.TrimSpace(seg)
		if strings.EqualFold(t, "ALL") {
			const posALL = 0
			if best == nil || si < best.si || (si == best.si && posALL < best.pos) {
				best = &hit{si, posALL, "全部"}
			}
			continue
		}
		u := strings.ToUpper(seg)
		for _, x := range []struct {
			sub, disp string
		}{
			{"DFT", "德非图"},
			{"LC", "宇海"},
		} {
			idx := strings.Index(u, x.sub)
			if idx < 0 {
				continue
			}
			if best == nil || si < best.si || (si == best.si && idx < best.pos) {
				best = &hit{si, idx, x.disp}
			}
		}
	}
	if best == nil {
		return ""
	}
	return best.val
}
