package envtags

import (
	"os"
	"strings"
)

func globalFallback() string {
	for _, key := range []string{
		"CLOUD_FITTER_ENV_TAG_KEY",
		"ENVIRONMENT_TAG_KEY",
		"environment",
	} {
		if k := strings.TrimSpace(os.Getenv(key)); k != "" {
			return k
		}
	}
	return ""
}

// ECSKey 读取用于 ECS 的标签键；空表示不填充 env_tag_value。
func ECSKey() string {
	if k := strings.TrimSpace(os.Getenv("CLOUD_FITTER_ECS_ENV_TAG_KEY")); k != "" {
		return k
	}
	return globalFallback()
}

// RDSKey 用于 RDS。
func RDSKey() string {
	if k := strings.TrimSpace(os.Getenv("CLOUD_FITTER_RDS_ENV_TAG_KEY")); k != "" {
		return k
	}
	return globalFallback()
}

// RedisKey 用于 Redis / DCS。
func RedisKey() string {
	if k := strings.TrimSpace(os.Getenv("CLOUD_FITTER_REDIS_ENV_TAG_KEY")); k != "" {
		return k
	}
	return globalFallback()
}
