package clusterenvscratch

import (
	"context"
	"strings"
)

// scratchKey 将 CCE HTTP JSON 的补充字段挂在 context 上，避免改动 protobuf。
type scratchKey struct{}

// Ensure 在未携带 scratch 时为 ctx 包装一层 map[string]string。
// cce.BySystemName 等对同一 baseCtx 下多次 List 时需在最外层 ancestor 调用，保证多条账号共用同一 scratch。
func Ensure(ctx context.Context) context.Context {
	if _, ok := ctx.Value(scratchKey{}).(map[string]string); ok {
		return ctx
	}
	m := make(map[string]string)
	return context.WithValue(ctx, scratchKey{}, m)
}

// Set 记录集群 UID（metadata.uid）对应的环境展示串；与 ECS/EIP 的 env_tag_value 一致。
func Set(ctx context.Context, clusterUID, envDisplay string) {
	m, ok := ctx.Value(scratchKey{}).(map[string]string)
	if !ok || m == nil {
		return
	}
	id := strings.TrimSpace(clusterUID)
	if id == "" {
		return
	}
	m[id] = strings.TrimSpace(envDisplay)
}

// GetMap 读取 scratch；可能为 nil（未 Ensure）。
func GetMap(ctx context.Context) map[string]string {
	m, _ := ctx.Value(scratchKey{}).(map[string]string)
	return m
}
