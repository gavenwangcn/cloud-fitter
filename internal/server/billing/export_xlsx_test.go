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
