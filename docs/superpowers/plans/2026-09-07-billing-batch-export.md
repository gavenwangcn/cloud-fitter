# Billing Batch Export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在账单汇总页支持按全部云账号配置 × 起止月份批量查询费用明细，经后端生成单表 Excel 并下载。

**Architecture:** 新增 `POST /apis/billing/batch-export`；handler 用 `configstore.Store.List()` 枚举账号，按月调用现有 `billing.ListSummary`（华为超时重试 1 次），过滤消费合计为 0 的行后用 excelize 写单表返回。前端仅在账单页于云账号下拉后增加起止月与导出按钮。

**Tech Stack:** Go 1.23、excelize/v2、现有 jsonapi + billing.ListSummary、React/Umi/Ant Design DatePicker

**Spec:** `docs/superpowers/specs/2026-09-07-billing-batch-export-design.md`

## Global Constraints

- 账号范围：与 `/apis/configs` 同源的全部云账号（`store.List()`）
- Excel：单表；列：序号、云类型、账号/范围、账单月份、资源大类、消费合计、币种、汇总行数
- 行序：账号（config id 升序）→ 月份升序 → ListSummary 明细顺序
- 仅导出 `TotalConsumeAmount != 0` 的行（proto3 float64；0 对应前端 omitempty 显示「—」）
- 失败跳过并继续；结束打失败汇总日志；无数据仍返回带表头空 xlsx
- 华为云超时错误重试 1 次；不改变现有 `by-account` 单次查询语义
- 客户端取消：检测到 `ctx.Done()` 则中止并返回错误，不返回半成品文件
- 不改造 gRPC BillingService；不给其他资源页加导出 UI
- 提交 git：仅在用户明确要求时执行 plan 中的 commit 步骤（默认跳过）

---

## File Structure

| 文件 | 职责 |
|------|------|
| `internal/server/billing/month_range.go` | 解析/展开 `YYYY-MM` 闭区间 |
| `internal/server/billing/month_range_test.go` | 月份展开单测 |
| `internal/server/billing/retry.go` | 超时判定 + 华为 `ListSummary` 重试包装 |
| `internal/server/billing/retry_test.go` | 超时判定与重试单测 |
| `internal/server/billing/export_xlsx.go` | 将明细行写成 excelize 单表字节 |
| `internal/server/billing/export_xlsx_test.go` | Excel 表头/过滤/序号单测 |
| `internal/server/jsonapi/billing_batch_export.go` | HTTP handler：校验、循环、日志、写响应 |
| `main.go` | 注册 `POST /apis/billing/batch-export` |
| `go.mod` / `go.sum` | 增加 `github.com/xuri/excelize/v2` |
| `cloud-fitter-web/src/components/CloudAccountBar/index.tsx` | 可选 `extra` 插槽 |
| `cloud-fitter-web/src/pages/billing/service.ts` | `exportBillingBatch` blob 请求 |
| `cloud-fitter-web/src/pages/billing/index.tsx` | 起止月 + 导出按钮与下载 |

---

### Task 1: 月份闭区间工具

**Files:**
- Create: `internal/server/billing/month_range.go`
- Create: `internal/server/billing/month_range_test.go`

**Interfaces:**
- Produces:
  - `func ParseYearMonth(s string) (time.Time, error)` — 解析 `YYYY-MM`（东八区月初）
  - `func MonthRangeInclusive(start, end string) ([]string, error)` — 返回 `["2026-01","2026-02",...]`；start>end 或格式非法返回 error

- [ ] **Step 1: Write the failing test**

Create `internal/server/billing/month_range_test.go`:

```go
package billing

import (
	"reflect"
	"testing"
)

func TestMonthRangeInclusive(t *testing.T) {
	got, err := MonthRangeInclusive("2026-01", "2026-03")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []string{"2026-01", "2026-02", "2026-03"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestMonthRangeInclusive_CrossYear(t *testing.T) {
	got, err := MonthRangeInclusive("2025-11", "2026-01")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []string{"2025-11", "2025-12", "2026-01"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestMonthRangeInclusive_StartAfterEnd(t *testing.T) {
	_, err := MonthRangeInclusive("2026-05", "2026-01")
	if err == nil {
		t.Fatal("expected error when start > end")
	}
}

func TestMonthRangeInclusive_BadFormat(t *testing.T) {
	_, err := MonthRangeInclusive("2026-1", "2026-02")
	if err == nil {
		t.Fatal("expected error for bad format")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/billing/ -run TestMonthRangeInclusive -count=1`

Expected: FAIL — `MonthRangeInclusive` undefined

- [ ] **Step 3: Write minimal implementation**

Create `internal/server/billing/month_range.go`:

```go
package billing

import (
	"strings"
	"time"

	"github.com/pkg/errors"
)

func ParseYearMonth(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.UTC
	}
	t, err := time.ParseInLocation("2006-01", s, loc)
	if err != nil {
		return time.Time{}, errors.Wrapf(err, "invalid month %q, want YYYY-MM", s)
	}
	return t, nil
}

// MonthRangeInclusive 返回 [start, end] 闭区间内每个 YYYY-MM（升序）。
func MonthRangeInclusive(start, end string) ([]string, error) {
	st, err := ParseYearMonth(start)
	if err != nil {
		return nil, err
	}
	en, err := ParseYearMonth(end)
	if err != nil {
		return nil, err
	}
	if st.After(en) {
		return nil, errors.Errorf("startMonth %s after endMonth %s", start, end)
	}
	var out []string
	for cur := st; !cur.After(en); cur = cur.AddDate(0, 1, 0) {
		out = append(out, cur.Format("2006-01"))
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/billing/ -run TestMonthRangeInclusive -count=1`

Expected: PASS

- [ ] **Step 5: Commit（仅当用户要求时）**

```bash
git add internal/server/billing/month_range.go internal/server/billing/month_range_test.go
git commit -m "feat(billing): add inclusive YYYY-MM month range helper"
```

---

### Task 2: 超时判定与华为 ListSummary 重试

**Files:**
- Create: `internal/server/billing/retry.go`
- Create: `internal/server/billing/retry_test.go`

**Interfaces:**
- Consumes: `ListSummary(ctx, *pbbilling.ListBillingSummaryReq)`
- Produces:
  - `func IsTimeoutErr(err error) bool`
  - `func ListSummaryForExport(ctx context.Context, req *pbbilling.ListBillingSummaryReq) (*pbbilling.ListBillingSummaryResp, error)`  
    — 非华为：直接 `ListSummary`；华为：失败且 `IsTimeoutErr` 则再调一次，并打日志

- [ ] **Step 1: Write the failing test**

Create `internal/server/billing/retry_test.go`:

```go
package billing

import (
	"errors"
	"testing"
)

func TestIsTimeoutErr(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("something else"), false},
		{errors.New("net/http: request canceled (Client.Timeout exceeded while awaiting headers)"), true},
		{errors.New("i/o timeout"), true},
		{errors.New("context deadline exceeded"), true},
		{errors.New("Timeout: read tcp"), true},
	}
	for _, c := range cases {
		if got := IsTimeoutErr(c.err); got != c.want {
			t.Errorf("IsTimeoutErr(%v)=%v want %v", c.err, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/billing/ -run TestIsTimeoutErr -count=1`

Expected: FAIL — `IsTimeoutErr` undefined

- [ ] **Step 3: Implement IsTimeoutErr + ListSummaryForExport**

Create `internal/server/billing/retry.go`:

```go
package billing

import (
	"context"
	"strings"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbbilling"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/golang/glog"
)

func IsTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "timeout") ||
		strings.Contains(s, "deadline exceeded") ||
		strings.Contains(s, "i/o timeout")
}

// ListSummaryForExport 供批量导出使用：华为云超时错误重试 1 次；其它云不重试。
func ListSummaryForExport(ctx context.Context, req *pbbilling.ListBillingSummaryReq) (*pbbilling.ListBillingSummaryResp, error) {
	resp, err := ListSummary(ctx, req)
	if err == nil {
		return resp, nil
	}
	if req == nil || req.Provider != pbtenant.CloudProvider_huawei || !IsTimeoutErr(err) {
		return nil, err
	}
	glog.Warningf("billing batch-export huawei timeout, retry once account=%s month=%s err=%v",
		req.AccountName, req.BillingCycle, err)
	resp2, err2 := ListSummary(ctx, req)
	if err2 != nil {
		glog.Errorf("billing batch-export huawei retry fail account=%s month=%s err=%v",
			req.AccountName, req.BillingCycle, err2)
		return nil, err2
	}
	glog.Infof("billing batch-export huawei retry ok account=%s month=%s",
		req.AccountName, req.BillingCycle)
	return resp2, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/server/billing/ -run "TestIsTimeoutErr|TestMonthRangeInclusive" -count=1`

Expected: PASS

- [ ] **Step 5: Commit（仅当用户要求时）**

```bash
git add internal/server/billing/retry.go internal/server/billing/retry_test.go
git commit -m "feat(billing): retry huawei ListSummary once on timeout for export"
```

---

### Task 3: Excel 单表构建 + 消费过滤

**Files:**
- Create: `internal/server/billing/export_xlsx.go`
- Create: `internal/server/billing/export_xlsx_test.go`
- Modify: `go.mod`, `go.sum`（`go get github.com/xuri/excelize/v2`）

**Interfaces:**
- Produces:
  - `type ExportRow struct { Provider int32; AccountName, BillingCycle, Category, Currency string; TotalConsumeAmount float64; SourceRowCount int32 }`
  - `func ProviderLabelCN(p int32) string` — `0阿里云 1腾讯云 2华为云 3AWS`，其它 `云(%d)`
  - `func BuildBillingExportXLSX(rows []ExportRow) ([]byte, error)` — 单表「费用明细」；跳过 `TotalConsumeAmount == 0`；序号从 1 起

- [ ] **Step 1: Add excelize dependency**

Run:

```bash
go get github.com/xuri/excelize/v2@latest
```

Expected: `go.mod` / `go.sum` 更新成功

- [ ] **Step 2: Write the failing test**

Create `internal/server/billing/export_xlsx_test.go`:

```go
package billing

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestBuildBillingExportXLSX_FiltersZeroAndOrders(t *testing.T) {
	in := []ExportRow{
		{Provider: 2, AccountName: "A1", BillingCycle: "2026-01", Category: "ECS", TotalConsumeAmount: 10.5, Currency: "CNY", SourceRowCount: 1},
		{Provider: 2, AccountName: "A1", BillingCycle: "2026-01", Category: "云监控", TotalConsumeAmount: 0, Currency: "CNY", SourceRowCount: 1},
		{Provider: 2, AccountName: "A1", BillingCycle: "2026-02", Category: "RDS", TotalConsumeAmount: 20, Currency: "CNY", SourceRowCount: 2},
	}
	raw, err := BuildBillingExportXLSX(in)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	sheet := "费用明细"
	// header
	h, _ := f.GetCellValue(sheet, "A1")
	if h != "序号" {
		t.Fatalf("header A1=%q", h)
	}
	// row1 = ECS
	v, _ := f.GetCellValue(sheet, "E2")
	if v != "ECS" {
		t.Fatalf("E2=%q want ECS", v)
	}
	amt, _ := f.GetCellValue(sheet, "F2")
	if amt != "10.50" {
		t.Fatalf("F2=%q want 10.50", amt)
	}
	// row2 = RDS (zero 云监控 skipped)
	v2, _ := f.GetCellValue(sheet, "E3")
	if v2 != "RDS" {
		t.Fatalf("E3=%q want RDS (zero row should be skipped)", v2)
	}
	// no 4th data row
	v3, _ := f.GetCellValue(sheet, "E4")
	if v3 != "" {
		t.Fatalf("unexpected E4=%q", v3)
	}
	cloud, _ := f.GetCellValue(sheet, "B2")
	if cloud != "华为云" {
		t.Fatalf("B2=%q want 华为云", cloud)
	}
}

func TestBuildBillingExportXLSX_EmptyStillHasHeader(t *testing.T) {
	raw, err := BuildBillingExportXLSX(nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	h, _ := f.GetCellValue("费用明细", "A1")
	if h != "序号" {
		t.Fatalf("header missing: %q", h)
	}
	v, _ := f.GetCellValue("费用明细", "A2")
	if v != "" {
		t.Fatalf("expected no data rows, got A2=%q", v)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/server/billing/ -run TestBuildBillingExportXLSX -count=1`

Expected: FAIL — `BuildBillingExportXLSX` undefined

- [ ] **Step 4: Implement export_xlsx.go**

```go
package billing

import (
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"
)

type ExportRow struct {
	Provider           int32
	AccountName        string
	BillingCycle       string
	Category           string
	TotalConsumeAmount float64
	Currency           string
	SourceRowCount     int32
}

func ProviderLabelCN(p int32) string {
	switch p {
	case 0:
		return "阿里云"
	case 1:
		return "腾讯云"
	case 2:
		return "华为云"
	case 3:
		return "AWS"
	default:
		return fmt.Sprintf("云(%d)", p)
	}
}

func BuildBillingExportXLSX(rows []ExportRow) ([]byte, error) {
	f := excelize.NewFile()
	sheet := "费用明细"
	idx, err := f.NewSheet(sheet)
	if err != nil {
		return nil, err
	}
	_ = f.SetActiveSheet(idx)
	_ = f.DeleteSheet("Sheet1")

	headers := []string{"序号", "云类型", "账号/范围", "账单月份", "资源大类", "消费合计", "币种", "汇总行数"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}

	seq := 0
	for _, r := range rows {
		if r.TotalConsumeAmount == 0 {
			continue
		}
		seq++
		vals := []interface{}{
			seq,
			ProviderLabelCN(r.Provider),
			r.AccountName,
			r.BillingCycle,
			r.Category,
			fmt.Sprintf("%.2f", r.TotalConsumeAmount),
			r.Currency,
			r.SourceRowCount,
		}
		for i, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(i+1, seq+1)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/server/billing/ -run "TestBuildBillingExportXLSX|TestIsTimeoutErr|TestMonthRangeInclusive" -count=1`

Expected: PASS

- [ ] **Step 6: Commit（仅当用户要求时）**

```bash
git add go.mod go.sum internal/server/billing/export_xlsx.go internal/server/billing/export_xlsx_test.go
git commit -m "feat(billing): build single-sheet billing export xlsx"
```

---

### Task 4: HTTP batch-export handler + 路由

**Files:**
- Create: `internal/server/jsonapi/billing_batch_export.go`
- Modify: `main.go`（在 `by-system-id` case 旁增加路由）

**Interfaces:**
- Consumes: `store.List()`, `MonthRangeInclusive`, `ListSummaryForExport`, `BuildBillingExportXLSX`, `writeBillingErr`
- Produces: `func BillingBatchExport(w http.ResponseWriter, r *http.Request, store *configstore.Store)`

- [ ] **Step 1: Implement handler**

Create `internal/server/jsonapi/billing_batch_export.go`:

```go
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
```

- [ ] **Step 2: Register route in main.go**

In the switch alongside other billing routes, add:

```go
case r.URL.Path == "/apis/billing/batch-export" && r.Method == http.MethodPost:
	jsonapi.BillingBatchExport(w, r, store)
```

Place immediately after the `by-system-id` case.

- [ ] **Step 3: Compile check**

Run: `go build -o NUL .`（Windows）或 `go build -o /dev/null .`

Expected: 编译成功

- [ ] **Step 4: Unit tests still pass**

Run: `go test ./internal/server/billing/ -count=1 -run "TestMonthRangeInclusive|TestIsTimeoutErr|TestBuildBillingExportXLSX"`

Expected: PASS

- [ ] **Step 5: Commit（仅当用户要求时）**

```bash
git add internal/server/jsonapi/billing_batch_export.go main.go
git commit -m "feat(billing): add POST /apis/billing/batch-export xlsx download"
```

---

### Task 5: CloudAccountBar 增加 extra 插槽

**Files:**
- Modify: `cloud-fitter-web/src/components/CloudAccountBar/index.tsx`

**Interfaces:**
- Produces: `CloudAccountBarProps.extra?: React.ReactNode` — 渲染在云账号 `Select` 之后

- [ ] **Step 1: Add `extra` prop**

Update props and render:

```tsx
export interface CloudAccountBarProps {
  onQuery: (provider: number, accountName: string) => void;
  onQueryBySystem?: (systemName: string) => void;
  accountOnly?: boolean;
  onClear?: () => void;
  /** 渲染在「云账号」选择器之后（如账单批量导出控件） */
  extra?: React.ReactNode;
}

const CloudAccountBar: React.FC<CloudAccountBarProps> = ({
  onQuery,
  onQueryBySystem,
  accountOnly = false,
  onClear,
  extra,
}) => {
  // ... existing state/effects unchanged ...
  return (
    <Space style={{ marginBottom: 16 }} align="center" wrap>
      {/* existing 系统名称 + 云账号 Select 不变 */}
      {/* after 云账号 Select: */}
      {extra}
    </Space>
  );
};
```

确保：其它页面不传 `extra` 时 UI 与现网一致（仅多一个可选 `wrap` 以容纳账单页加长控件，可接受）。

- [ ] **Step 2: 目视确认其它引用无需改动**

`ecs/rds/...` 等仍 `<CloudAccountBar ... />`，不传 `extra`。

- [ ] **Step 3: Commit（仅当用户要求时）**

```bash
git add cloud-fitter-web/src/components/CloudAccountBar/index.tsx
git commit -m "feat(web): allow CloudAccountBar extra slot after account select"
```

---

### Task 6: 账单页起止月 + 导出下载

**Files:**
- Modify: `cloud-fitter-web/src/pages/billing/service.ts`
- Modify: `cloud-fitter-web/src/pages/billing/index.tsx`

**Interfaces:**
- Consumes: `CloudAccountBar.extra`、`POST /apis/billing/batch-export`
- Produces: `exportBillingBatch(startMonth: string, endMonth: string): Promise<void>`（内部触发下载）

- [ ] **Step 1: Add export API helper**

Append to `cloud-fitter-web/src/pages/billing/service.ts`:

```ts
import { API_REQUEST_TIMEOUT_MS } from '@/constants/requestTimeout';
import { request } from 'umi';

// ... existing queryBillingByAccount / queryBillingBySystem ...

/** 批量导出全部云账号在 [startMonth, endMonth] 的费用明细 xlsx */
export async function exportBillingBatch(startMonth: string, endMonth: string) {
  const blob: Blob = await request('/apis/billing/batch-export', {
    method: 'POST',
    data: { startMonth, endMonth },
    responseType: 'blob',
    timeout: API_REQUEST_TIMEOUT_MS,
    getResponse: false,
  });
  const filename = `billing-export-${startMonth}-${endMonth}.xlsx`;
  const url = window.URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  window.URL.revokeObjectURL(url);
}
```

若项目 `request` 在 `responseType: 'blob'` 时实际返回结构不同（例如带 `data`），按 Umi 3 实际行为调整为取 `Blob` 本体；错误时若 blob 实为 JSON error，可 `blob.text()` 后 `JSON.parse` 取 `error` 字段再 `message.error`。

错误处理建议封装：

```ts
export async function exportBillingBatch(startMonth: string, endMonth: string) {
  try {
    const data: any = await request('/apis/billing/batch-export', {
      method: 'POST',
      data: { startMonth, endMonth },
      responseType: 'blob',
      timeout: API_REQUEST_TIMEOUT_MS,
    });
    const blob: Blob = data instanceof Blob ? data : new Blob([data]);
    if (blob.type && blob.type.includes('application/json')) {
      const text = await blob.text();
      let msg = '导出失败';
      try {
        const j = JSON.parse(text);
        msg = j.error || j.errMsg || msg;
      } catch {
        msg = text || msg;
      }
      throw new Error(msg);
    }
    const filename = `billing-export-${startMonth}-${endMonth}.xlsx`;
    const url = window.URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    a.remove();
    window.URL.revokeObjectURL(url);
  } catch (e: any) {
    throw e;
  }
}
```

- [ ] **Step 2: Wire UI in billing/index.tsx**

增加 state 与控件（示意，贴入现有组件）：

```tsx
const [exportStart, setExportStart] = useState<Dayjs | null>(null);
const [exportEnd, setExportEnd] = useState<Dayjs | null>(null);
const [exporting, setExporting] = useState(false);

const canExport =
  !!exportStart &&
  !!exportEnd &&
  !exportStart.isAfter(exportEnd, 'month');

const onBatchExport = async () => {
  if (!canExport || !exportStart || !exportEnd) return;
  setExporting(true);
  try {
    await exportBillingBatch(exportStart.format('YYYY-MM'), exportEnd.format('YYYY-MM'));
    message.success('导出成功');
  } catch (e: any) {
    message.error(e?.message || '批量导出失败');
  } finally {
    setExporting(false);
  }
};

// CloudAccountBar:
<CloudAccountBar
  onQuery={...}
  onQueryBySystem={...}
  onClear={clearTable}
  extra={
    <>
      <span>开始时间：</span>
      <DatePicker
        picker="month"
        value={exportStart}
        onChange={(d) => setExportStart(d)}
        allowClear
      />
      <span>结束时间：</span>
      <DatePicker
        picker="month"
        value={exportEnd}
        onChange={(d) => setExportEnd(d)}
        allowClear
      />
      <Button type="primary" disabled={!canExport} loading={exporting} onClick={() => void onBatchExport()}>
        导出
      </Button>
    </>
  }
/>
```

从 `./service` 增加 `import { exportBillingBatch } from './service';`（若该页此前未直接 import service，仅通过 model，则新增此 import）。

- [ ] **Step 3: Manual smoke（本地）**

1. 打开账单汇总页，确认云账号后出现开始/结束/导出
2. 未选齐或 start>end 时导出禁用
3. 选合法区间点击导出 → 下载 xlsx，打开确认列与过滤
4. 查看后端日志含 `billing batch-export start/ok/fail/done`

- [ ] **Step 4: Commit（仅当用户要求时）**

```bash
git add cloud-fitter-web/src/pages/billing/service.ts cloud-fitter-web/src/pages/billing/index.tsx cloud-fitter-web/src/components/CloudAccountBar/index.tsx
git commit -m "feat(web): billing page batch export date range and download"
```

---

## Spec Coverage Checklist

| Spec 要求 | Task |
|-----------|------|
| 云账号后起止月 + 导出按钮 | Task 5–6 |
| 按钮启用：非空且 start≤end | Task 6 |
| 全部配置账号 × 月份循环 | Task 4 |
| 复用 ListSummary / 与图 2 一致 | Task 2–4 |
| 消费合计不为空才导出 | Task 3（`!= 0`） |
| 单表 Excel + 列/行序 | Task 3–4 |
| 空数据带表头 | Task 3 |
| 失败跳过 + 明细日志 | Task 4 |
| 华为超时重试 1 次 | Task 2 |
| 取消不返回半成品 | Task 4（ctx 检查后 writeBillingErr） |
| 不改其它页 CloudAccountBar 行为 | Task 5（optional extra） |

## Self-Review Notes

- 无 TBD/placeholder
- `ListSummaryForExport` / `MonthRangeInclusive` / `BuildBillingExportXLSX` / `BillingBatchExport` / `exportBillingBatch` 命名前后一致
- proto3 `TotalConsumeAmount` 用 `!= 0` 过滤，与规格「对齐前端 —」一致

---

**Plan complete and saved to `docs/superpowers/plans/2026-09-07-billing-batch-export.md`.**

**两种执行方式：**

1. **Subagent-Driven（推荐）** — 每个 Task 派一个新子代理，Task 间做审查，迭代快  
2. **Inline Execution** — 本会话按 executing-plans 顺序执行，带检查点  

选哪一种？
