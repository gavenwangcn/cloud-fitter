package cmdb

import (
	"context"
	"time"

	"github.com/cloud-fitter/cloud-fitter/internal/cmdb/synclog"
)

// SystemSyncOutcome 单系统同步结果（供落库与 Run 汇总）。
type SystemSyncOutcome struct {
	SystemID   string
	SystemName string
	Skipped    bool
	SkipReason string
	EarlyError string
	Stats      systemSyncStats
}

func (o SystemSyncOutcome) synclogStats() synclog.SystemStats {
	return synclog.SystemStats{
		SystemNode:       toSynclogComponent(o.Stats.SystemNode),
		K8s:              toSynclogComponent(o.Stats.K8s),
		Host:             toSynclogComponent(o.Stats.Host),
		MiddlewareByType: toSynclogMiddlewareMap(o.Stats.MiddlewareByType),
		EIP:              toSynclogComponent(o.Stats.EIP),
		ELB:              toSynclogComponent(o.Stats.ELB),
		WAFDomain:        toSynclogComponent(o.Stats.WAFDomain),
		Certificate:      toSynclogComponent(o.Stats.Certificate),
		Billing:          toSynclogComponent(o.Stats.Billing),
	}
}

func (o SystemSyncOutcome) toSystemDetail() synclog.SystemDetail {
	status := "success"
	errMsg := ""
	switch {
	case o.Skipped:
		status = "skipped"
		errMsg = o.SkipReason
	case o.EarlyError != "":
		status = "error"
		errMsg = o.EarlyError
	case o.Stats.totalErrors() > 0:
		status = "failed"
	default:
		status = "success"
	}
	return synclog.SystemDetailFrom(o.SystemID, o.SystemName, status, errMsg, o.synclogStats())
}

func toSynclogComponent(st componentSyncStats) synclog.ComponentStats {
	return synclog.ComponentStats{
		Added:     st.Added,
		Updated:   st.Updated,
		Skipped:   st.Skipped,
		Deleted:   st.Deleted,
		Errors:    st.Errors,
		FailedIDs: append([]string(nil), st.FailedIDs...),
	}
}

func toSynclogMiddlewareMap(m map[string]componentSyncStats) map[string]synclog.ComponentStats {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]synclog.ComponentStats, len(m))
	for k, v := range m {
		out[k] = toSynclogComponent(v)
	}
	return out
}

func (st systemSyncStats) totalErrors() int {
	n := st.SystemNode.Errors + st.K8s.Errors + st.Host.Errors + st.Middleware.Errors +
		st.EIP.Errors + st.ELB.Errors + st.WAFDomain.Errors + st.Certificate.Errors + st.Billing.Errors
	for _, c := range st.MiddlewareByType {
		n += c.Errors
	}
	return n
}

func (s *Syncer) syncLogStore() *synclog.Store {
	if s == nil || s.Store == nil {
		return nil
	}
	return synclog.NewStore(s.Store.SQLDB())
}

func (s *Syncer) persistSyncRun(ctx context.Context, trigger synclog.TriggerType, manualSystemID, manualSystemName string, started time.Time, outcomes []SystemSyncOutcome, runErr error) int64 {
	logStore := s.syncLogStore()
	if logStore == nil {
		return 0
	}
	systems := make([]synclog.SystemDetail, 0, len(outcomes))
	for _, o := range outcomes {
		systems = append(systems, o.toSystemDetail())
	}
	summary := synclog.BuildRunSummary(trigger, manualSystemID, manualSystemName, started, time.Now(), systems)
	errMsg := ""
	if runErr != nil {
		errMsg = runErr.Error()
		if summary.Status == "success" {
			summary.Status = "failed"
		}
	}
	runID, err := logStore.InsertRun(ctx, summary, errMsg)
	if err != nil {
		return 0
	}
	return runID
}
