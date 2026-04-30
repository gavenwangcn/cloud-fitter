package configstore

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
)

type createSystemBody struct {
	Name        string  `json:"name"`
	EnglishName string  `json:"englishName"`
	ShortName   string  `json:"shortName"`
	Intro       string  `json:"intro"`
	SystemID    string  `json:"systemId"`
	OnlineTime  string  `json:"onlineTime"`
	Status      string  `json:"status"`
	AccountIDs  []int64 `json:"accountIds"`
}

type updateSystemBody struct {
	ID          int64   `json:"id"`
	SystemID    string  `json:"systemId"`
	EnglishName string  `json:"englishName"`
	ShortName   string  `json:"shortName"`
	Intro       string  `json:"intro"`
	OnlineTime  string  `json:"onlineTime"`
	Status      string  `json:"status"`
	AccountIDs  []int64 `json:"accountIds"`
}

// SystemHTTPHandler 处理 GET/POST/PUT/DELETE /apis/systems。
func SystemHTTPHandler(store *Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		glog.Infof("systems api begin method=%s path=%s", r.Method, r.URL.Path)
		defer glog.Infof("systems api end method=%s path=%s elapsed=%v", r.Method, r.URL.Path, time.Since(start))
		switch r.Method {
		case http.MethodGet:
			page := ParseIntDefault(r.URL.Query().Get("page"), 1)
			pageSize := ParseIntDefault(r.URL.Query().Get("pageSize"), 50)
			rows, total, err := store.ListSystemsPaged(page, pageSize)
			if err != nil {
				glog.Warningf("systems api list failed err=%v", err)
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			glog.Infof("systems api list ok rows=%d total=%d", len(rows), total)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"systems": rows, "total": total})
		case http.MethodPost:
			var body createSystemBody
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				glog.Warningf("systems api decode failed err=%v", err)
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			body.Name = strings.TrimSpace(body.Name)
			body.EnglishName = strings.TrimSpace(body.EnglishName)
			body.ShortName = strings.TrimSpace(body.ShortName)
			body.Intro = strings.TrimSpace(body.Intro)
			body.SystemID = strings.TrimSpace(body.SystemID)
			body.OnlineTime = strings.TrimSpace(body.OnlineTime)
			body.Status = strings.TrimSpace(body.Status)
			if body.Name == "" || body.Intro == "" || body.SystemID == "" || body.OnlineTime == "" || body.Status == "" || len(body.AccountIDs) == 0 {
				glog.Warningf("systems api validation failed name=%q system_id=%q online_time=%q status=%q account_ids=%d",
					body.Name, body.SystemID, body.OnlineTime, body.Status, len(body.AccountIDs))
				writeErr(w, http.StatusBadRequest, errors.New("系统名称、系统功能简介、系统ID、上线时间、状态、关联账号均不能为空"))
				return
			}
			if body.Status != "上线" && body.Status != "建设中" && body.Status != "下线" {
				glog.Warningf("systems api validation failed invalid status=%q", body.Status)
				writeErr(w, http.StatusBadRequest, errors.New("状态仅支持：上线/建设中/下线"))
				return
			}
			exist, err := store.HasSystemID(body.SystemID)
			if err != nil {
				glog.Warningf("systems api check system id failed system_id=%s err=%v", body.SystemID, err)
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			if exist {
				glog.Warningf("systems api create conflict system_id=%s", body.SystemID)
				writeErr(w, http.StatusConflict, errors.New("ID信息重复"))
				return
			}
			if err := store.CreateSystem(body.Name, body.EnglishName, body.ShortName, body.Intro, body.SystemID, body.OnlineTime, body.Status, body.AccountIDs); err != nil {
				if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					glog.Warningf("systems api create conflict(unique) system_id=%s err=%v", body.SystemID, err)
					writeErr(w, http.StatusConflict, errors.New("ID信息重复"))
					return
				}
				glog.Warningf("systems api create failed name=%s system_id=%s err=%v", body.Name, body.SystemID, err)
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			glog.Infof("systems api create ok name=%s system_id=%s account_ids=%d status=%s online_time=%s",
				body.Name, body.SystemID, len(body.AccountIDs), body.Status, body.OnlineTime)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case http.MethodPut:
			var body updateSystemBody
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			if body.ID < 1 {
				writeErr(w, http.StatusBadRequest, errors.New("id 无效"))
				return
			}
			body.Intro = strings.TrimSpace(body.Intro)
			body.SystemID = strings.TrimSpace(body.SystemID)
			body.EnglishName = strings.TrimSpace(body.EnglishName)
			body.ShortName = strings.TrimSpace(body.ShortName)
			body.OnlineTime = strings.TrimSpace(body.OnlineTime)
			body.Status = strings.TrimSpace(body.Status)
			if body.SystemID == "" || body.Intro == "" || body.OnlineTime == "" || body.Status == "" || len(body.AccountIDs) == 0 {
				writeErr(w, http.StatusBadRequest, errors.New("系统ID、简介、上线时间、状态、关联账号均不能为空"))
				return
			}
			if body.Status != "上线" && body.Status != "建设中" && body.Status != "下线" {
				writeErr(w, http.StatusBadRequest, errors.New("状态仅支持：上线/建设中/下线"))
				return
			}
			if err := store.UpdateSystemByID(body.ID, body.SystemID, body.EnglishName, body.ShortName, body.Intro, body.OnlineTime, body.Status, body.AccountIDs); err != nil {
				if err.Error() == "系统不存在或 id 错误" {
					writeErr(w, http.StatusNotFound, err)
					return
				}
				if err.Error() == "ID信息重复" {
					writeErr(w, http.StatusConflict, err)
					return
				}
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			glog.Infof("systems api update ok id=%d", body.ID)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case http.MethodDelete:
			id := int64(ParseIntDefault(r.URL.Query().Get("id"), 0))
			if id < 1 {
				writeErr(w, http.StatusBadRequest, errors.New("id 无效"))
				return
			}
			if err := store.DeleteSystemByID(id); err != nil {
				if err.Error() == "系统不存在或 id 错误" {
					writeErr(w, http.StatusNotFound, err)
					return
				}
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			glog.Infof("systems api delete ok id=%d", id)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}
