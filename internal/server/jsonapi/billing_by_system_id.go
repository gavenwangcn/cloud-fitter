package jsonapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbbilling"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/configstore"
	billingsvc "github.com/cloud-fitter/cloud-fitter/internal/server/billing"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"
)

type billingBySystemIDBody struct {
	SystemID     string `json:"systemId"`
	BillingMonth string `json:"billingMonth"`
}

// BillingSummaryBySystemID POST /apis/billing/by-system-id
// body: {"systemId":"D-000002","billingMonth":"2026-05"}（月份可空，默认当月东八区）
// 返回系统下各关联云账号的分账单汇总，供前端弹框列表展示 account_name。
func BillingSummaryBySystemID(w http.ResponseWriter, r *http.Request, store *configstore.Store) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if store == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "配置存储不可用"})
		return
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	var body billingBySystemIDBody
	if err := json.Unmarshal(b, &body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	sid := strings.TrimSpace(body.SystemID)
	if sid == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "systemId 不能为空"})
		return
	}
	sysRow, err := store.SystemBySystemID(sid)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	accounts, err := store.AccountsBySystemName(sysRow.Name)
	if err != nil {
		glog.Errorf("billing by-system-id: accounts err=%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	loc, lerr := time.LoadLocation("Asia/Shanghai")
	if lerr != nil {
		loc = time.UTC
	}
	cycle := strings.TrimSpace(body.BillingMonth)
	if cycle == "" {
		cycle = time.Now().In(loc).Format("2006-01")
	}

	ctx := r.Context()
	type acctPart struct {
		AccountName string                 `json:"accountName"`
		Provider    int32                  `json:"provider"`
		Summary     map[string]interface{} `json:"summary"`
	}
	out := struct {
		SystemID     string     `json:"systemId"`
		SystemName   string     `json:"systemName"`
		BillingMonth string     `json:"billingMonth"`
		Accounts     []acctPart `json:"accounts"`
	}{
		SystemID:     sid,
		SystemName:   sysRow.Name,
		BillingMonth: cycle,
		Accounts:     nil,
	}

	for _, acc := range accounts {
		resp, err := billingsvc.ListSummary(ctx, &pbbilling.ListBillingSummaryReq{
			Provider:     pbtenant.CloudProvider(acc.Provider),
			BillingCycle: cycle,
			AccountName:  acc.Name,
		})
		if err != nil {
			glog.Errorf("billing by-system-id ListSummary account=%s err=%v", acc.Name, err)
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": errors.Wrapf(err, "账号 %s 拉取账单失败", acc.Name).Error()})
			return
		}
		raw, err := protojson.Marshal(resp)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		var summ map[string]interface{}
		if err := json.Unmarshal(raw, &summ); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		out.Accounts = append(out.Accounts, acctPart{
			AccountName: acc.Name,
			Provider:    acc.Provider,
			Summary:     summ,
		})
	}

	_ = json.NewEncoder(w).Encode(out)
}
