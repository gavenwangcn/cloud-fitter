package jsonapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbbilling"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/configstore"
	"github.com/cloud-fitter/cloud-fitter/internal/server/billing"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

type billingBatchExportBody struct {
	StartMonth string `json:"startMonth"`
	EndMonth   string `json:"endMonth"`
}

// BillingBatchExport POST /apis/billing/batch-export
// body: {"startMonth":"YYYY-MM","endMonth":"YYYY-MM"}
// 枚举全部云账号配置 × 月份，复用 ListSummary；失败跳过；返回单表 xlsx。
func BillingBatchExport(w http.ResponseWriter, r *http.Request, store *configstore.Store) {
	startAll := time.Now()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeBillingErr(w, http.StatusBadRequest, errors.Wrap(err, "read body"))
		return
	}
	var body billingBatchExportBody
	if err := json.Unmarshal(raw, &body); err != nil {
		writeBillingErr(w, http.StatusBadRequest, errors.Wrap(err, "decode billing batch-export body"))
		return
	}
	body.StartMonth = strings.TrimSpace(body.StartMonth)
	body.EndMonth = strings.TrimSpace(body.EndMonth)
	if body.StartMonth == "" || body.EndMonth == "" {
		writeBillingErr(w, http.StatusBadRequest, errors.New("startMonth and endMonth are required (YYYY-MM)"))
		return
	}
	months, err := billing.MonthRangeInclusive(body.StartMonth, body.EndMonth)
	if err != nil {
		writeBillingErr(w, http.StatusBadRequest, err)
		return
	}
	if store == nil {
		writeBillingErr(w, http.StatusInternalServerError, errors.New("config store is nil"))
		return
	}
	configs, err := store.List()
	if err != nil {
		writeBillingErr(w, http.StatusInternalServerError, errors.Wrap(err, "list cloud configs"))
		return
	}

	ctx := r.Context()
	queries := len(configs) * len(months)
	glog.Infof("billing batch-export start accounts=%d start=%s end=%s months=%d queries=%d",
		len(configs), body.StartMonth, body.EndMonth, len(months), queries)

	var exportRows []billing.ExportRow
	var failures []string
	successN, failN := 0, 0

	for _, cfg := range configs {
		for _, month := range months {
			if err := ctx.Err(); err != nil {
				writeBillingErr(w, http.StatusRequestTimeout, errors.Wrap(err, "client canceled billing batch-export"))
				return
			}
			qStart := time.Now()
			req := &pbbilling.ListBillingSummaryReq{
				Provider:     pbtenant.CloudProvider(cfg.Provider),
				AccountName:  cfg.Name,
				BillingCycle: month,
			}
			resp, err := billing.ListSummaryForExport(ctx, req)
			elapsed := time.Since(qStart)
			if err != nil {
				failN++
				msg := fmt.Sprintf("account=%s provider=%d month=%s err=%v", cfg.Name, cfg.Provider, month, err)
				failures = append(failures, msg)
				glog.Errorf("billing batch-export fail %s elapsed=%v", msg, elapsed)
				continue
			}
			kept := 0
			for _, row := range resp.GetRows() {
				if row == nil || row.GetTotalConsumeAmount() == 0 {
					continue
				}
				kept++
				exportRows = append(exportRows, billing.ExportRow{
					Provider:           int32(row.GetProvider()),
					AccountName:        row.GetAccountName(),
					BillingCycle:       row.GetBillingCycle(),
					Category:           row.GetCategory(),
					TotalConsumeAmount: row.GetTotalConsumeAmount(),
					Currency:           row.GetCurrency(),
					SourceRowCount:     row.GetSourceRowCount(),
				})
			}
			successN++
			glog.Infof("billing batch-export ok account=%s provider=%d month=%s rows=%d kept=%d elapsed=%v",
				cfg.Name, cfg.Provider, month, len(resp.GetRows()), kept, elapsed)
		}
	}

	glog.Infof("billing batch-export done success=%d fail=%d exportRows=%d failures=%v elapsed=%v",
		successN, failN, len(exportRows), failures, time.Since(startAll))

	xlsx, err := billing.BuildBillingExportXLSX(exportRows)
	if err != nil {
		writeBillingErr(w, http.StatusInternalServerError, errors.Wrap(err, "build xlsx"))
		return
	}
	filename := fmt.Sprintf("billing-export-%s-%s.xlsx", body.StartMonth, body.EndMonth)
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(xlsx)
}
