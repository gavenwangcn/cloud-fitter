package synclog

import "time"

// TriggerType 同步触发方式。
type TriggerType string

const (
	TriggerScheduled TriggerType = "scheduled"
	TriggerManual    TriggerType = "manual"
)

// FailureDetail 单条失败明细（资源 ID + 原因）。
type FailureDetail struct {
	ResourceID string `json:"resource_id,omitempty"`
	Reason     string `json:"reason"`
}

// ResourceFailure 单类资源同步失败明细（系统维度）。
type ResourceFailure struct {
	FailCount int             `json:"fail_count"`
	FailedIDs []string        `json:"failed_ids,omitempty"`
	Failures  []FailureDetail `json:"failures,omitempty"`
}

// ResourceFailBySystem 某系统下某类资源的失败明细。
type ResourceFailBySystem struct {
	SystemID   string          `json:"system_id"`
	SystemName string          `json:"system_name"`
	FailCount  int             `json:"fail_count"`
	FailedIDs  []string        `json:"failed_ids,omitempty"`
	Failures   []FailureDetail `json:"failures,omitempty"`
}

// ResourceFailTotal 某类资源失败汇总（含跨系统明细，便于排查）。
type ResourceFailTotal struct {
	FailCount int                    `json:"fail_count"`
	Items     []ResourceFailBySystem `json:"items,omitempty"`
}

// SystemDetail 单系统同步结果。
type SystemDetail struct {
	SystemID   string                     `json:"system_id"`
	SystemName string                     `json:"system_name"`
	Status     string                     `json:"status"` // success | failed | skipped | error
	Error      string                     `json:"error,omitempty"`
	Resources  map[string]ResourceFailure `json:"resources,omitempty"`
}

// RunSummary 一次同步批次汇总（写入 cmdb_sync_run）。
type RunSummary struct {
	ID              int64          `json:"id"`
	TriggerType     TriggerType    `json:"trigger_type"`
	ManualSystemID  string         `json:"manual_system_id,omitempty"`
	ManualSystemName string        `json:"manual_system_name,omitempty"`
	StartedAt       time.Time      `json:"started_at"`
	FinishedAt      time.Time      `json:"finished_at"`
	TotalSystems    int            `json:"total_systems"`
	SuccessSystems  int            `json:"success_systems"`
	FailedSystems   int            `json:"failed_systems"`
	SkippedSystems  int            `json:"skipped_systems"`
	Status          string         `json:"status"` // success | partial | failed
	ResourceTotals  map[string]ResourceFailTotal `json:"resource_fail_totals"`
	Systems         []SystemDetail `json:"systems"`
}
