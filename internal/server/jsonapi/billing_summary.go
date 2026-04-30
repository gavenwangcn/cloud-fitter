package jsonapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbbilling"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/server/billing"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

type billingSummaryBody struct {
	Provider     int32  `json:"provider"`
	AccountName  string `json:"accountName"`
	SystemName   string `json:"systemName"`
	BillingCycle string `json:"billingCycle"`
}

func decodeBillingSummaryBody(r *http.Request) (billingSummaryBody, error) {
	var body billingSummaryBody
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return body, err
	}
	if err := json.Unmarshal(b, &body); err != nil {
		return body, err
	}
	return body, nil
}

// BillingSummaryByAccount POST /apis/billing/by-account
// 与 ECS 等一致：可选 systemName 合并系统内多账号；billingCycle 默认当月（东八区）。
func BillingSummaryByAccount(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBillingSummaryBody(r)
	if err != nil {
		writeBillingErr(w, http.StatusBadRequest, errors.Wrap(err, "decode billing request body"))
		return
	}
	loc, lerr := time.LoadLocation("Asia/Shanghai")
	if lerr != nil {
		loc = time.UTC
	}
	cycle := strings.TrimSpace(body.BillingCycle)
	if cycle == "" {
		cycle = time.Now().In(loc).Format("2006-01")
	}
	ctx := r.Context()

	if body.SystemName != "" {
		accounts, err := resolveSystemAccounts(body.SystemName)
		if err != nil {
			writeBillingErr(w, http.StatusInternalServerError, errors.Wrapf(err, "resolve system accounts failed system=%q", body.SystemName))
			return
		}
		if len(accounts) == 0 {
			writeProtoJSON(w, &pbbilling.ListBillingSummaryResp{Currency: "CNY"}, nil)
			return
		}
		parts := make([]*pbbilling.ListBillingSummaryResp, 0, len(accounts))
		for _, acc := range accounts {
			resp, err := billing.ListSummary(ctx, &pbbilling.ListBillingSummaryReq{
				Provider:     pbtenant.CloudProvider(acc.Provider),
				BillingCycle: cycle,
				AccountName:  acc.AccountName,
			})
			if err != nil {
				writeBillingErr(w, http.StatusInternalServerError, errors.Wrapf(
					err,
					"billing summary failed system=%q provider=%v account=%q cycle=%q",
					body.SystemName, pbtenant.CloudProvider(acc.Provider), acc.AccountName, cycle,
				))
				return
			}
			parts = append(parts, resp)
		}
		merged := billing.MergeCategorySummaries(cycle, "（系统内多账号）", parts)
		writeProtoJSON(w, merged, nil)
		return
	}

	resp, err := billing.ListSummary(ctx, &pbbilling.ListBillingSummaryReq{
		Provider:     pbtenant.CloudProvider(body.Provider),
		BillingCycle: cycle,
		AccountName:  body.AccountName,
	})
	if err != nil {
		writeBillingErr(w, http.StatusInternalServerError, errors.Wrapf(
			err,
			"billing summary failed provider=%v account=%q cycle=%q",
			pbtenant.CloudProvider(body.Provider), body.AccountName, cycle,
		))
		return
	}
	writeProtoJSON(w, resp, nil)
}

func writeBillingErr(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	msg := err.Error()
	// 打印完整错误链，便于云上直接定位真实 OpenAPI 返回。
	glog.Errorf("billing api error: %+v", err)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":  msg,
		"errMsg": msg,
		"resMsg": msg,
	})
}
