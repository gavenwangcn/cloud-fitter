package jsonapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbbilling"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/billingbatchexport"
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
// 异步：立即返回文件名，后台生成 xlsx 写入 batch-export 目录。
func BillingBatchExport(w http.ResponseWriter, r *http.Request, store *configstore.Store) {
	body, err := decodeBillingBatchExportBody(r)
	if err != nil {
		writeBillingErr(w, http.StatusBadRequest, err)
		return
	}
	if store == nil {
		writeBillingErr(w, http.StatusInternalServerError, errors.New("config store is nil"))
		return
	}
	months, err := billing.MonthRangeInclusive(body.StartMonth, body.EndMonth)
	if err != nil {
		writeBillingErr(w, http.StatusBadRequest, err)
		return
	}
	configs, err := store.List()
	if err != nil {
		writeBillingErr(w, http.StatusInternalServerError, errors.Wrap(err, "list cloud configs"))
		return
	}

	now := time.Now()
	filename, err := billingbatchexport.NewFilename(now)
	if err != nil {
		writeBillingErr(w, http.StatusInternalServerError, errors.Wrap(err, "allocate export filename"))
		return
	}

	go runBillingBatchExportJob(context.Background(), configs, months, body.StartMonth, body.EndMonth, filename)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"filename": filename,
		"message":  "导出任务已启动，完成后可在文件列表中下载",
	})
}

// BillingBatchExportListFiles GET /apis/billing/batch-export/files
func BillingBatchExportListFiles(w http.ResponseWriter, r *http.Request) {
	files, err := billingbatchexport.ListXLSX()
	if err != nil {
		writeBillingErr(w, http.StatusInternalServerError, errors.Wrap(err, "list export files"))
		return
	}
	if files == nil {
		files = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string][]string{"files": files})
}

// BillingBatchExportDownload GET /apis/billing/batch-export/download?filename=...
func BillingBatchExportDownload(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("filename"))
	abs, err := billingbatchexport.ResolveFile(name)
	if err != nil {
		writeBillingErr(w, http.StatusBadRequest, err)
		return
	}
	f, err := os.Open(abs)
	if err != nil {
		if os.IsNotExist(err) {
			writeBillingErr(w, http.StatusNotFound, errors.New("export file not found"))
			return
		}
		writeBillingErr(w, http.StatusInternalServerError, errors.Wrap(err, "open export file"))
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename="+jsonQuoteFilename(name))
	http.ServeContent(w, r, name, mustModTime(f, abs), f)
}

func decodeBillingBatchExportBody(r *http.Request) (billingBatchExportBody, error) {
	var body billingBatchExportBody
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return body, errors.Wrap(err, "read body")
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return body, errors.Wrap(err, "decode billing batch-export body")
	}
	body.StartMonth = strings.TrimSpace(body.StartMonth)
	body.EndMonth = strings.TrimSpace(body.EndMonth)
	if body.StartMonth == "" || body.EndMonth == "" {
		return body, errors.New("startMonth and endMonth are required (YYYY-MM)")
	}
	return body, nil
}

func runBillingBatchExportJob(
	ctx context.Context,
	configs []configstore.Row,
	months []string,
	startMonth, endMonth, filename string,
) {
	startAll := time.Now()
	queries := len(configs) * len(months)
	glog.Infof("billing batch-export async start file=%s accounts=%d start=%s end=%s months=%d queries=%d dir=%s",
		filename, len(configs), startMonth, endMonth, len(months), queries, billingbatchexport.ExportDir())

	var exportRows []billing.ExportRow
	var failures []string
	successN, failN := 0, 0

	for _, cfg := range configs {
		for _, month := range months {
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
				glog.Errorf("billing batch-export async fail %s elapsed=%v", msg, elapsed)
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
			glog.Infof("billing batch-export async ok account=%s provider=%d month=%s rows=%d kept=%d elapsed=%v",
				cfg.Name, cfg.Provider, month, len(resp.GetRows()), kept, elapsed)
		}
	}

	xlsx, err := billing.BuildBillingExportXLSX(exportRows)
	if err != nil {
		glog.Errorf("billing batch-export async build xlsx failed file=%s err=%v", filename, err)
		return
	}

	dir := billingbatchexport.ExportDir()
	if err := billingbatchexport.EnsureDir(); err != nil {
		glog.Errorf("billing batch-export async mkdir failed dir=%s err=%v", dir, err)
		return
	}
	partPath := filepath.Join(dir, filename+".part")
	finalPath := filepath.Join(dir, filename)
	if err := os.WriteFile(partPath, xlsx, 0o644); err != nil {
		glog.Errorf("billing batch-export async write failed path=%s err=%v", partPath, err)
		return
	}
	if err := os.Rename(partPath, finalPath); err != nil {
		glog.Errorf("billing batch-export async rename failed part=%s final=%s err=%v", partPath, finalPath, err)
		_ = os.Remove(partPath)
		return
	}

	glog.Infof("billing batch-export async done file=%s success=%d fail=%d exportRows=%d failures=%v elapsed=%v",
		filename, successN, failN, len(exportRows), failures, time.Since(startAll))
}

func jsonQuoteFilename(name string) string {
	b, _ := json.Marshal(name)
	// json.Marshal adds quotes — Content-Disposition filename= expects quoted string
	return string(b)
}

func mustModTime(f *os.File, path string) time.Time {
	if st, err := f.Stat(); err == nil {
		return st.ModTime()
	}
	if st, err := os.Stat(path); err == nil {
		return st.ModTime()
	}
	return time.Now()
}
