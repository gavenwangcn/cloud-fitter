package resourcecache

import (
	"os"
	"strings"
	"time"
)

// SnapshotWorkerEnabled 为 false 时不启动定时快照拉取（默认 true）。环境变量：CLOUD_FITTER_RESOURCE_SNAPSHOT_ENABLE。
func SnapshotWorkerEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("CLOUD_FITTER_RESOURCE_SNAPSHOT_ENABLE")))
	switch v {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// SnapshotRunOnStartupFromEnv 为 true 时进程启动后约 30s 执行首轮快照；默认 false，避免首次启动即全量拉云写库。
// 环境变量：CLOUD_FITTER_RESOURCE_SNAPSHOT_RUN_ON_STARTUP
func SnapshotRunOnStartupFromEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("CLOUD_FITTER_RESOURCE_SNAPSHOT_RUN_ON_STARTUP")))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// SnapshotIntervalFromEnv 全量快照拉取周期，默认 3h。示例：3h、180m。小于 1m 则回退默认。
func SnapshotIntervalFromEnv() time.Duration {
	s := strings.TrimSpace(os.Getenv("CLOUD_FITTER_RESOURCE_SNAPSHOT_INTERVAL"))
	if s == "" {
		return 3 * time.Hour
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < time.Minute {
		return 3 * time.Hour
	}
	return d
}

// CMDBUseResourceSnapshotFromEnv 为 true 时，CMDB 凌晨同步仅从 MySQL 快照表读数据，不再调用云 List 接口。
func CMDBUseResourceSnapshotFromEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("CLOUD_FITTER_CMDB_USE_RESOURCE_SNAPSHOT")))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
