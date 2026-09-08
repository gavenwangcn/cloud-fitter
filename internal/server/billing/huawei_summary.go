package billing

import (
	"context"

	"github.com/golang/glog"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbbilling"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/billingagg"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

func huaweiListBillingSummary(ctx context.Context, tenant tenanter.Tenanter, billingCycle string) (resp *pbbilling.ListBillingSummaryResp, err error) {
	lines, currency, err := huaweiFetchMonthlySumLines(ctx, tenant, billingCycle)
	if err != nil {
		return nil, err
	}

	type agg struct {
		sum float64
		n   int32
	}
	m := make(map[string]*agg)
	var grand float64

	for _, line := range lines {
		cat := billingagg.HuaweiCategoryFromServiceType(line.ServiceTypeCode)
		if cat == "其他" {
			glog.Infof("huawei billing ShowCustomerMonthlySum -> 其他: account=%s bill_cycle=%s service_type_code=%q consume_amount=%.2f currency=%s",
				tenant.AccountName(), billingCycle, line.ServiceTypeCode, line.ConsumeAmount, currency)
		}
		a := m[cat]
		if a == nil {
			a = &agg{}
			m[cat] = a
		}
		a.sum += line.ConsumeAmount
		a.n++
		grand += line.ConsumeAmount
	}

	cur := currency
	if cur == "" {
		cur = "CNY"
	}

	rows := make([]*pbbilling.BillingCategoryRow, 0, len(m))
	seen := make(map[string]bool)
	for _, cat := range billingagg.BillingCategoryDisplayOrder {
		if a, ok := m[cat]; ok && a != nil {
			rows = append(rows, &pbbilling.BillingCategoryRow{
				Provider:           pbtenant.CloudProvider_huawei,
				AccountName:        tenant.AccountName(),
				BillingCycle:       billingCycle,
				Category:           cat,
				TotalConsumeAmount: billingagg.RoundMoney2(a.sum),
				Currency:           cur,
				SourceRowCount:     a.n,
			})
			seen[cat] = true
		}
	}
	for cat, a := range m {
		if seen[cat] || a == nil {
			continue
		}
		rows = append(rows, &pbbilling.BillingCategoryRow{
			Provider:           pbtenant.CloudProvider_huawei,
			AccountName:        tenant.AccountName(),
			BillingCycle:       billingCycle,
			Category:           cat,
			TotalConsumeAmount: billingagg.RoundMoney2(a.sum),
			Currency:           cur,
			SourceRowCount:     a.n,
		})
	}

	return &pbbilling.ListBillingSummaryResp{
		Rows:              rows,
		GrandTotalConsume: billingagg.RoundMoney2(grand),
		Currency:          cur,
	}, nil
}
