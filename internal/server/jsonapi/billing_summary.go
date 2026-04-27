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
		http.Error(w, err.Error(), http.StatusBadRequest)
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
			writeProtoJSON(w, nil, err)
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
				writeProtoJSON(w, nil, err)
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
	writeProtoJSON(w, resp, err)
}
