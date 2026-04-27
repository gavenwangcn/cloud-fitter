package billing

import (
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbbilling"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/billingagg"
)

type catProvKey struct {
	p pbtenant.CloudProvider
	c string
}

// MergeCategorySummaries 将多个账号的汇总结果合并；相同云类型、相同大类合并为一行（多账号 ECS 费用相加）。
// 若云类型不同则分开展示（避免不同币种/口径混加）。
func MergeCategorySummaries(billingCycle, accountLabel string, parts []*pbbilling.ListBillingSummaryResp) *pbbilling.ListBillingSummaryResp {
	type agg struct {
		sum      float64
		n        int32
		currency string
	}
	m := make(map[catProvKey]*agg)
	provSet := make(map[pbtenant.CloudProvider]struct{})
	for _, r := range parts {
		if r == nil {
			continue
		}
		cur := r.Currency
		if cur == "" {
			cur = "CNY"
		}
		for _, row := range r.Rows {
			if row == nil {
				continue
			}
			provSet[row.Provider] = struct{}{}
			k := catProvKey{p: row.Provider, c: row.Category}
			a := m[k]
			if a == nil {
				a = &agg{currency: cur}
				m[k] = a
			}
			a.sum += row.TotalConsumeAmount
			a.n += row.SourceRowCount
		}
	}
	multiCloud := len(provSet) > 1
	var grand float64
	rows := make([]*pbbilling.BillingCategoryRow, 0, len(m))
	order := []string{"ECS", "RDS", "DCS", "DMS", "CCE", "MongoDB", "其他"}
	seen := make(map[catProvKey]bool)
	for _, cat := range order {
		for k, a := range m {
			if k.c != cat || a == nil {
				continue
			}
			rows = append(rows, &pbbilling.BillingCategoryRow{
				Provider:            k.p,
				AccountName:         accountLabel,
				BillingCycle:        billingCycle,
				Category:            categoryLabel(cat, k.p, multiCloud),
				TotalConsumeAmount:  billingagg.RoundMoney2(a.sum),
				Currency:            a.currency,
				SourceRowCount:      a.n,
			})
			grand += a.sum
			seen[k] = true
		}
	}
	for k, a := range m {
		if seen[k] || a == nil {
			continue
		}
		rows = append(rows, &pbbilling.BillingCategoryRow{
			Provider:            k.p,
			AccountName:         accountLabel,
			BillingCycle:        billingCycle,
			Category:            categoryLabel(k.c, k.p, multiCloud),
			TotalConsumeAmount:  billingagg.RoundMoney2(a.sum),
			Currency:            a.currency,
			SourceRowCount:      a.n,
		})
		grand += a.sum
	}
	currency := "CNY"
	if len(parts) > 0 && parts[0] != nil && parts[0].Currency != "" {
		currency = parts[0].Currency
	}
	return &pbbilling.ListBillingSummaryResp{
		Rows:              rows,
		GrandTotalConsume: billingagg.RoundMoney2(grand),
		Currency:          currency,
	}
}

func categoryLabel(cat string, p pbtenant.CloudProvider, multiCloud bool) string {
	if multiCloud {
		return cat + "（" + providerShortCN(p) + "）"
	}
	return cat
}

func providerShortCN(p pbtenant.CloudProvider) string {
	switch p {
	case pbtenant.CloudProvider_ali:
		return "阿里云"
	case pbtenant.CloudProvider_tencent:
		return "腾讯云"
	case pbtenant.CloudProvider_huawei:
		return "华为云"
	case pbtenant.CloudProvider_aws:
		return "AWS"
	default:
		return p.String()
	}
}
