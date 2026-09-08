package jsonapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/server/billing"
	"github.com/pkg/errors"
)

type billingOtherBreakdownBody struct {
	Provider     int32  `json:"provider"`
	AccountName  string `json:"accountName"`
	SystemName   string `json:"systemName"`
	BillingMonth string `json:"billingMonth"`
	BillingCycle string `json:"billingCycle"`
}

func decodeBillingOtherBreakdownBody(r *http.Request) (billingOtherBreakdownBody, error) {
	var body billingOtherBreakdownBody
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return body, err
	}
	if err := json.Unmarshal(b, &body); err != nil {
		return body, err
	}
	return body, nil
}

func effectiveBillingMonthOther(body billingOtherBreakdownBody) string {
	if m := strings.TrimSpace(body.BillingMonth); m != "" {
		return m
	}
	return strings.TrimSpace(body.BillingCycle)
}

// BillingOtherBreakdown POST /apis/billing/other-breakdown
// 返回华为云主表「其他」行下的费用明细（按文件存储/云备份等大类拆分）。
func BillingOtherBreakdown(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBillingOtherBreakdownBody(r)
	if err != nil {
		writeBillingErr(w, http.StatusBadRequest, errors.Wrap(err, "decode other breakdown body"))
		return
	}
	cycle := effectiveBillingMonthOther(body)
	if cycle == "" {
		writeBillingErr(w, http.StatusBadRequest, errors.New("billingMonth is required"))
		return
	}

	resolveSystem := func(systemName string) ([]billing.OtherBreakdownAccount, error) {
		sc, err := resolveSystemListScope(systemName)
		if err != nil {
			return nil, err
		}
		out := make([]billing.OtherBreakdownAccount, 0, len(sc.Accounts))
		for _, acc := range sc.Accounts {
			out = append(out, billing.OtherBreakdownAccount{
				Provider:    pbtenant.CloudProvider(acc.Provider),
				AccountName: acc.AccountName,
			})
		}
		return out, nil
	}

	resp, err := billing.ListOtherBreakdown(
		r.Context(),
		pbtenant.CloudProvider(body.Provider),
		cycle,
		body.AccountName,
		body.SystemName,
		resolveSystem,
	)
	if err != nil {
		writeBillingErr(w, http.StatusInternalServerError, errors.Wrap(err, "other breakdown failed"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
