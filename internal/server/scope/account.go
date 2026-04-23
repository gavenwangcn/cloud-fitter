package scope

import "context"

type ctxKey int

const accountNameKey ctxKey = iota

// WithAccountName 将账户名（配置中的 name）写入 context，供 ECS/RDS/Redis List 按单账号过滤。
func WithAccountName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, accountNameKey, name)
}

// AccountName 返回要限制的账户名；空表示不限制（该云下全部账户）。
func AccountName(ctx context.Context) string {
	s, _ := ctx.Value(accountNameKey).(string)
	return s
}
