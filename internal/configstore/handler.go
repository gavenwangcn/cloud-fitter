package configstore

import (
	"encoding/json"
	"net/http"
	"strings"

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
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch r.Method {
		case http.MethodGet:
			rows, err := store.List()
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"configs": rows})
		case http.MethodPost:
			var body createBody
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			body.Name = strings.TrimSpace(body.Name)
			body.AccessID = strings.TrimSpace(body.AccessID)
			body.AccessSecret = strings.TrimSpace(body.AccessSecret)
			if body.Name == "" || body.AccessID == "" || body.AccessSecret == "" {
				writeErr(w, http.StatusBadRequest, errors.New("name, accessId, accessSecret required"))
				return
			}
			if err := store.Create(body.Provider, body.Name, body.AccessID, body.AccessSecret); err != nil {
				if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					writeErr(w, http.StatusConflict, errors.New("name already exists"))
					return
				}
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
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
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
