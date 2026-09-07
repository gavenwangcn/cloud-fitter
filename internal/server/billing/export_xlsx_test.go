package billing

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestBuildBillingExportXLSX_ChargesAndRefundsSheets(t *testing.T) {
	in := []ExportRow{
		{Provider: 2, AccountName: "A1", BillingCycle: "2026-01", Category: "ECS", TotalConsumeAmount: 10.5, Currency: "CNY", SourceRowCount: 1},
		{Provider: 2, AccountName: "A1", BillingCycle: "2026-01", Category: "云监控", TotalConsumeAmount: 0, Currency: "CNY", SourceRowCount: 1},
		{Provider: 2, AccountName: "A1", BillingCycle: "2026-02", Category: "RDS", TotalConsumeAmount: 20, Currency: "CNY", SourceRowCount: 2},
		{Provider: 2, AccountName: "A2", BillingCycle: "2026-06", Category: "负载均衡", TotalConsumeAmount: -832.33, Currency: "CNY", SourceRowCount: 2},
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

	h, _ := f.GetCellValue(ExportSheetCharges, "A1")
	if h != "序号" {
		t.Fatalf("charges header A1=%q", h)
	}
	v, _ := f.GetCellValue(ExportSheetCharges, "E2")
	if v != "ECS" {
		t.Fatalf("charges E2=%q want ECS", v)
	}
	v2, _ := f.GetCellValue(ExportSheetCharges, "E3")
	if v2 != "RDS" {
		t.Fatalf("charges E3=%q want RDS", v2)
	}
	if v4, _ := f.GetCellValue(ExportSheetCharges, "E4"); v4 != "" {
		t.Fatalf("charges should not contain refund row, E4=%q", v4)
	}

	hR, _ := f.GetCellValue(ExportSheetRefunds, "A1")
	if hR != "序号" {
		t.Fatalf("refunds header A1=%q", hR)
	}
	cat, _ := f.GetCellValue(ExportSheetRefunds, "E2")
	if cat != "负载均衡" {
		t.Fatalf("refunds E2=%q want 负载均衡", cat)
	}
	amt, _ := f.GetCellValue(ExportSheetRefunds, "F2")
	if amt != "-832.33" {
		t.Fatalf("refunds F2=%q want -832.33", amt)
	}
	acct, _ := f.GetCellValue(ExportSheetRefunds, "C2")
	if acct != "A2" {
		t.Fatalf("refunds C2=%q want A2", acct)
	}
}

func TestBuildBillingExportXLSX_EmptyStillHasBothSheetHeaders(t *testing.T) {
	raw, err := BuildBillingExportXLSX(nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	for _, sheet := range []string{ExportSheetCharges, ExportSheetRefunds} {
		h, _ := f.GetCellValue(sheet, "A1")
		if h != "序号" {
			t.Fatalf("sheet %s header missing: %q", sheet, h)
		}
		v, _ := f.GetCellValue(sheet, "A2")
		if v != "" {
			t.Fatalf("sheet %s expected no data rows, got A2=%q", sheet, v)
		}
	}
}
