package synclog

import (
	"strings"
	"time"
)

// FailureEntry 与 cmdb.syncFailureEntry 对齐。
type FailureEntry struct {
	ResourceID string
	Reason     string
}

// ComponentStats 与 cmdb.componentSyncStats 字段对齐，避免 synclog 依赖 cmdb 包循环引用。
type ComponentStats struct {
	Added     int
	Updated   int
	Skipped   int
	Deleted   int
	Errors    int
	FailedIDs []string
	Failures  []FailureEntry
}

// SystemStats 单系统各组件统计。
type SystemStats struct {
	SystemNode       ComponentStats
	K8s              ComponentStats
	Host             ComponentStats
	MiddlewareByType map[string]ComponentStats
	EIP              ComponentStats
	ELB              ComponentStats
	WAFDomain        ComponentStats
	Certificate      ComponentStats
	Billing          ComponentStats
}

func (s SystemStats) TotalErrors() int {
	n := s.SystemNode.Errors + s.K8s.Errors + s.Host.Errors + s.EIP.Errors +
		s.ELB.Errors + s.WAFDomain.Errors + s.Certificate.Errors + s.Billing.Errors
	for _, st := range s.MiddlewareByType {
		n += st.Errors
	}
	return n
}

func failureDetails(entries []FailureEntry) []FailureDetail {
	if len(entries) == 0 {
		return nil
	}
	out := make([]FailureDetail, 0, len(entries))
	for _, e := range entries {
		reason := strings.TrimSpace(e.Reason)
		if reason == "" {
			reason = "unknown error"
		}
		out = append(out, FailureDetail{
			ResourceID: strings.TrimSpace(e.ResourceID),
			Reason:     reason,
		})
	}
	return out
}

func resourceFailure(st ComponentStats) (ResourceFailure, bool) {
	if st.Errors <= 0 {
		return ResourceFailure{}, false
	}
	failures := failureDetails(st.Failures)
	ids := dedupeNonEmpty(st.FailedIDs)
	if len(ids) == 0 && len(failures) > 0 {
		for _, f := range failures {
			if f.ResourceID != "" {
				ids = append(ids, f.ResourceID)
			}
		}
		ids = dedupeNonEmpty(ids)
	}
	return ResourceFailure{
		FailCount: st.Errors,
		FailedIDs: ids,
		Failures:  failures,
	}, true
}

func systemResources(stats SystemStats) map[string]ResourceFailure {
	out := map[string]ResourceFailure{}
	put := func(name string, st ComponentStats) {
		if rf, ok := resourceFailure(st); ok {
			out[name] = rf
		}
	}
	put("SystemNode", stats.SystemNode)
	put("CCE", stats.K8s)
	put("ECS", stats.Host)
	for mwType, st := range stats.MiddlewareByType {
		name := middlewareDisplayName(mwType)
		if rf, ok := resourceFailure(st); ok {
			if prev, exists := out[name]; exists {
				prev.FailCount += rf.FailCount
				prev.FailedIDs = dedupeNonEmpty(append(prev.FailedIDs, rf.FailedIDs...))
				prev.Failures = append(prev.Failures, rf.Failures...)
				out[name] = prev
			} else {
				out[name] = rf
			}
		}
	}
	put("EIP", stats.EIP)
	put("ELB", stats.ELB)
	put("WAF", stats.WAFDomain)
	put("Certificate", stats.Certificate)
	put("Billing", stats.Billing)
	if len(out) == 0 {
		return nil
	}
	return out
}

func middlewareDisplayName(mwType string) string {
	switch strings.TrimSpace(mwType) {
	case "RDS_INS":
		return "RDS"
	case "DCS_REDIS":
		return "DCS"
	case "DMS_ROCKETMQ":
		return "Kafka"
	default:
		if mwType == "" {
			return "Middleware"
		}
		return mwType
	}
}

func dedupeNonEmpty(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// buildResourceFailTotals 汇总各资源失败总数，并附带「系统 + 资源 ID」明细。
func buildResourceFailTotals(systems []SystemDetail) map[string]ResourceFailTotal {
	out := map[string]ResourceFailTotal{}
	for _, sys := range systems {
		for resName, rf := range sys.Resources {
			if rf.FailCount <= 0 {
				continue
			}
			total := out[resName]
			total.FailCount += rf.FailCount
			total.Items = append(total.Items, ResourceFailBySystem{
				SystemID:   sys.SystemID,
				SystemName: sys.SystemName,
				FailCount:  rf.FailCount,
				FailedIDs:  dedupeNonEmpty(rf.FailedIDs),
				Failures:   rf.Failures,
			})
			out[resName] = total
		}
	}
	if len(out) == 0 {
		return map[string]ResourceFailTotal{}
	}
	return out
}

// BuildRunSummary 由单系统结果列表构建批次汇总。
func BuildRunSummary(trigger TriggerType, manualSystemID, manualSystemName string, startedAt, finishedAt time.Time, systems []SystemDetail) RunSummary {
	var success, failed, skipped int
	for _, sys := range systems {
		switch sys.Status {
		case "success":
			success++
		case "skipped":
			skipped++
		default:
			failed++
		}
	}
	totals := buildResourceFailTotals(systems)
	status := "success"
	if failed > 0 {
		if success > 0 {
			status = "partial"
		} else {
			status = "failed"
		}
	}
	return RunSummary{
		TriggerType:      trigger,
		ManualSystemID:   manualSystemID,
		ManualSystemName: manualSystemName,
		StartedAt:        startedAt,
		FinishedAt:       finishedAt,
		TotalSystems:     len(systems),
		SuccessSystems:   success,
		FailedSystems:    failed,
		SkippedSystems:   skipped,
		Status:           status,
		ResourceTotals:   totals,
		Systems:          systems,
	}
}

// SystemDetailFrom 将单系统统计转为落库明细。
func SystemDetailFrom(systemID, systemName, status, errMsg string, stats SystemStats) SystemDetail {
	return SystemDetail{
		SystemID:   systemID,
		SystemName: systemName,
		Status:     status,
		Error:      strings.TrimSpace(errMsg),
		Resources:  systemResources(stats),
	}
}
