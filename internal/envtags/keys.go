package envtags

import (
	"os"
	"strings"
)

// defaultEnvTagKey 为各云资源共用的环境类标签在控制台/API 中常用的键名；可通过环境变量覆盖。
const defaultEnvTagKey = "environment"

// defaultNodeTagKey 为各云资源共用的「节点」类标签键名（CMDB 系统节点、列表「节点(标签)」）；可通过环境变量覆盖。
const defaultNodeTagKey = "node"

// UnifiedEnvTagKey 返回 ECS/RDS/Redis 等**共用**的环境标签键名（列表「环境(标签)」等）。
// 优先级：CLOUD_FITTER_ENV_TAG_KEY > ENVIRONMENT_TAG_KEY > 默认 "environment"。
// 仅当需对某一类资源单独指定键时，再使用下面的 CLOUD_FITTER_ECS/RDS/REDIS_ENV_TAG_KEY（兼容旧配置）。
func UnifiedEnvTagKey() string {
	if k := strings.TrimSpace(os.Getenv("CLOUD_FITTER_ENV_TAG_KEY")); k != "" {
		return k
	}
	if k := strings.TrimSpace(os.Getenv("ENVIRONMENT_TAG_KEY")); k != "" {
		return k
	}
	return defaultEnvTagKey
}

// ECSKey 用于 ECS 的标签键。若未单独设置 CLOUD_FITTER_ECS_ENV_TAG_KEY，则与 UnifiedEnvTagKey 一致。
func ECSKey() string {
	if k := strings.TrimSpace(os.Getenv("CLOUD_FITTER_ECS_ENV_TAG_KEY")); k != "" {
		return k
	}
	return UnifiedEnvTagKey()
}

// RDSKey 用于 RDS。
func RDSKey() string {
	if k := strings.TrimSpace(os.Getenv("CLOUD_FITTER_RDS_ENV_TAG_KEY")); k != "" {
		return k
	}
	return UnifiedEnvTagKey()
}

// RedisKey 用于 Redis / DCS。
func RedisKey() string {
	if k := strings.TrimSpace(os.Getenv("CLOUD_FITTER_REDIS_ENV_TAG_KEY")); k != "" {
		return k
	}
	return UnifiedEnvTagKey()
}

// NodeTagKey 返回「节点」标签键名；默认 "node"。配置：CLOUD_FITTER_NODE_TAG_KEY。
func NodeTagKey() string {
	if k := strings.TrimSpace(os.Getenv("CLOUD_FITTER_NODE_TAG_KEY")); k != "" {
		return k
	}
	return defaultNodeTagKey
}
