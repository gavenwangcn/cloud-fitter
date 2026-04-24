package cmdb

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/golang/glog"
)

type syncPostBody struct {
	SystemName string `json:"systemName"`
}

// SyncHTTPHandler 处理 POST /apis/cmdb/sync，body: {"systemName":"…"}。syncer 为 nil 时表示 CMDB 未配置。
func SyncHTTPHandler(syncer *Syncer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if syncer == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "CMDB 未配置，请设置 CLOUD_FITTER_CMDB_* 或启动参数 cmdb-base-url / cmdb-key / cmdb-secret"})
			return
		}
		var body syncPostBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		name := strings.TrimSpace(body.SystemName)
		if name == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "systemName 不能为空"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Hour)
		defer cancel()
		if err := syncer.SyncOneBySystemName(ctx, name); err != nil {
			msg := err.Error()
			glog.Warningf("cmdb single sync: %s", msg)
			code := http.StatusInternalServerError
			switch msg {
			case "CMDB中没有相同系统信息":
				code = http.StatusUnprocessableEntity
			case "未找到该系统":
				code = http.StatusNotFound
			case "本系统未关联云账号，无法同步", "系统名称不能为空":
				code = http.StatusBadRequest
			}
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
}
