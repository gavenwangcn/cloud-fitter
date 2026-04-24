package configstore

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
)

type reloadFn func() error

type createBody struct {
	Provider     int32  `json:"provider"`
	Name         string `json:"name"`
	AccessID     string `json:"accessId"`
	AccessSecret string `json:"accessSecret"`
}

// HTTPHandler 处理 GET/POST /apis/configs；创建成功后调用 reload 刷新内存租户。
func HTTPHandler(store *Store, reload reloadFn) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		glog.Infof("configs api begin method=%s path=%s", r.Method, r.URL.Path)
		defer glog.Infof("configs api end method=%s path=%s elapsed=%v", r.Method, r.URL.Path, time.Since(start))
		switch r.Method {
		case http.MethodGet:
			rows, err := store.List()
			if err != nil {
				glog.Warningf("configs api list failed err=%v", err)
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			glog.Infof("configs api list ok rows=%d", len(rows))
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"configs": rows})
		case http.MethodPost:
			var body createBody
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				glog.Warningf("configs api decode failed err=%v", err)
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			body.Name = strings.TrimSpace(body.Name)
			body.AccessID = strings.TrimSpace(body.AccessID)
			body.AccessSecret = strings.TrimSpace(body.AccessSecret)
			if body.Name == "" || body.AccessID == "" || body.AccessSecret == "" {
				glog.Warningf("configs api validation failed name=%q accessId_empty=%v accessSecret_empty=%v",
					body.Name, body.AccessID == "", body.AccessSecret == "")
				writeErr(w, http.StatusBadRequest, errors.New("name, accessId, accessSecret required"))
				return
			}
			if err := store.Create(body.Provider, body.Name, body.AccessID, body.AccessSecret); err != nil {
				if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					glog.Warningf("configs api create conflict provider=%d name=%s err=%v", body.Provider, body.Name, err)
					writeErr(w, http.StatusConflict, errors.New("name already exists"))
					return
				}
				glog.Warningf("configs api create failed provider=%d name=%s err=%v", body.Provider, body.Name, err)
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			glog.Infof("configs api create ok provider=%d name=%s", body.Provider, body.Name)
			if reload != nil {
				if err := reload(); err != nil {
					glog.Errorf("reload tenants after config create: %v", err)
					writeErr(w, http.StatusInternalServerError, err)
					return
				}
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func writeErr(w http.ResponseWriter, code int, err error) {
	glog.Warningf("configs/systems api response error code=%d err=%v", code, err)
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
