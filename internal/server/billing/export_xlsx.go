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
	f.SetActiveSheet(idx)
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
