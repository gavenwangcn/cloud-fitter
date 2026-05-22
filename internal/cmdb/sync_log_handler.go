package cmdb

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cloud-fitter/cloud-fitter/internal/cmdb/synclog"
)

// SyncRunsHTTPHandler GET /apis/cmdb/sync/runs?page=1&pageSize=20
func SyncRunsHTTPHandler(syncer *Syncer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if syncer == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "CMDB 未配置"})
			return
		}
		store := syncer.syncLogStore()
		if store == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "同步日志需要 MySQL，请配置 CLOUD_FITTER_DB_DRIVER=mysql"})
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
		items, total, err := store.ListRuns(r.Context(), page, pageSize)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total": total,
			"items": items,
		})
	})
}

// SyncRunByIDHTTPHandler GET /apis/cmdb/sync/runs/{id}
func SyncRunByIDHTTPHandler(syncer *Syncer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if syncer == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "CMDB 未配置"})
			return
		}
		store := syncer.syncLogStore()
		if store == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "同步日志需要 MySQL"})
			return
		}
		idStr := strings.TrimPrefix(r.URL.Path, "/apis/cmdb/sync/runs/")
		idStr = strings.Trim(strings.TrimSpace(idStr), "/")
		runID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || runID <= 0 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid run id"})
			return
		}
		item, err := store.GetRun(r.Context(), runID)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(item)
	})
}

// SyncLogStore 供测试或外部注入。
func (s *Syncer) SyncLogStore() *synclog.Store {
	return s.syncLogStore()
}

// PersistSyncRunForTest 暴露落库逻辑供单元测试。
func (s *Syncer) PersistSyncRunForTest(ctx context.Context, trigger synclog.TriggerType, manualSystemID, manualSystemName string, started time.Time, outcomes []SystemSyncOutcome, runErr error) int64 {
	return s.persistSyncRun(ctx, trigger, manualSystemID, manualSystemName, started, outcomes, runErr)
}
