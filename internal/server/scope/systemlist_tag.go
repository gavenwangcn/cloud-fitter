package scope

import (
	"context"
	"strings"
)

type systemListTagFilterKey struct{}

// WithSystemListTagFilter 在按「系统名称」拉取云资源列表时注入当前 CMDB system_id；供各云 ListDetail 按标签 system（见 envtags.SystemTagKey）过滤。
// systemID 为空时不过滤（兼容未配置系统行或旧调用路径）。
func WithSystemListTagFilter(ctx context.Context, systemID string) context.Context {
	return context.WithValue(ctx, systemListTagFilterKey{}, strings.TrimSpace(systemID))
}

// SystemListTagFilterMatches 若未注入 system_id 则恒为 true；否则：标签值为空则纳入；非空则须与注入的 system_id 完全一致。
func SystemListTagFilterMatches(ctx context.Context, systemTagValue string) bool {
	sid, ok := ctx.Value(systemListTagFilterKey{}).(string)
	if !ok || sid == "" {
		return true
	}
	v := strings.TrimSpace(systemTagValue)
	if v == "" {
		return true
	}
	return v == sid
}

// FilterSliceBySystemListTag 按系统列表上下文过滤切片：未注入 system_id 时原样返回；
// 否则仅保留 SystemListTagFilterMatches 为 true 的项（无系统标签保留，有标签须与当前 system_id 一致）。
func FilterSliceBySystemListTag[T any](ctx context.Context, items []T, systemTagOf func(T) string) []T {
	sid, ok := ctx.Value(systemListTagFilterKey{}).(string)
	if !ok || sid == "" || systemTagOf == nil {
		return items
	}
	out := make([]T, 0, len(items))
	for _, it := range items {
		if SystemListTagFilterMatches(ctx, systemTagOf(it)) {
			out = append(out, it)
		}
	}
	return out
}
