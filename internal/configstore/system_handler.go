package configstore

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/pkg/errors"
)

type createSystemBody struct {
	Name       string  `json:"name"`
	Intro      string  `json:"intro"`
	SystemID   string  `json:"systemId"`
	AccountIDs []int64 `json:"accountIds"`
}

// SystemHTTPHandler 处理 GET/POST /apis/systems。
func SystemHTTPHandler(store *Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch r.Method {
		case http.MethodGet:
			rows, err := store.ListSystems()
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"systems": rows})
		case http.MethodPost:
			var body createSystemBody
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			body.Name = strings.TrimSpace(body.Name)
			body.Intro = strings.TrimSpace(body.Intro)
			body.SystemID = strings.TrimSpace(body.SystemID)
			if body.Name == "" || body.Intro == "" || body.SystemID == "" || len(body.AccountIDs) == 0 {
				writeErr(w, http.StatusBadRequest, errors.New("系统名称、系统功能简介、系统ID、关联账号均不能为空"))
				return
			}
			exist, err := store.HasSystemID(body.SystemID)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			if exist {
				writeErr(w, http.StatusConflict, errors.New("ID信息重复"))
				return
			}
			if err := store.CreateSystem(body.Name, body.Intro, body.SystemID, body.AccountIDs); err != nil {
				if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					writeErr(w, http.StatusConflict, errors.New("ID信息重复"))
					return
				}
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}
