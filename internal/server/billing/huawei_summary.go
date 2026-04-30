package billing

import (
	"context"
	"strings"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
	bssv2 "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bss/v2"
	bssmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bss/v2/model"
	bssregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bss/v2/region"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbbilling"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/billingagg"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

func huaweiListBillingSummary(ctx context.Context, tenant tenanter.Tenanter, billingCycle string) (*pbbilling.ListBillingSummaryResp, error) {
	t, ok := tenant.(*tenanter.AccessKeyTenant)
	if !ok {
		return nil, errors.New("huawei billing: only AccessKeyTenant supported")
	}
	// BSS 属于华为云 global 服务，需使用 global.Credentials（官方 SDK 文档）。
	auth := global.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).Build()
	hc := bssv2.BssClientBuilder().WithRegion(bssregion.ValueOf("cn-north-1")).WithCredential(auth).Build()
	cli := bssv2.NewBssClient(hc)

	type agg struct {
		sum float64
		n   int32
	}
	m := make(map[string]*agg)
	var currency string
	var grand float64

	offset := int32(0)
	limit := int32(500)
	method := "oneself"
	for {
		req := &bssmodel.ShowCustomerMonthlySumRequest{
			BillCycle: billingCycle,
			Offset:    &offset,
			Limit:     &limit,
			Method:    &method,
		}
		resp, err := cli.ShowCustomerMonthlySum(req)
		if err != nil {
			return nil, errors.Wrap(err, "ShowCustomerMonthlySum")
		}
		if resp.Currency != nil && *resp.Currency != "" {
			currency = *resp.Currency
		}
		if resp.BillSums == nil {
			break
		}
		for _, row := range *resp.BillSums {
			svc := ""
			if row.ServiceTypeCode != nil {
				svc = *row.ServiceTypeCode
			}
			cat := billingagg.HuaweiCategoryFromServiceType(svc)
			amt := 0.0
			if row.ConsumeAmount != nil {
				amt = *row.ConsumeAmount
			}
			a := m[cat]
			if a == nil {
				a = &agg{}
				m[cat] = a
			}
			a.sum += amt
			a.n++
			grand += amt
		}
		batch := int32(len(*resp.BillSums))
		if resp.TotalCount == nil || batch == 0 || offset+batch >= *resp.TotalCount {
			break
		}
		offset += batch
	}

	cur := currency
	if cur == "" {
		cur = "CNY"
	}
	cur = strings.TrimSpace(cur)

	rows := make([]*pbbilling.BillingCategoryRow, 0, len(m))
	order := []string{"ECS", "RDS", "DCS", "DMS", "CCE", "其他"}
	seen := make(map[string]bool)
	for _, cat := range order {
		if a, ok := m[cat]; ok && a != nil {
			rows = append(rows, &pbbilling.BillingCategoryRow{
				Provider:            pbtenant.CloudProvider_huawei,
				AccountName:         tenant.AccountName(),
				BillingCycle:        billingCycle,
				Category:            cat,
				TotalConsumeAmount:  billingagg.RoundMoney2(a.sum),
				Currency:            cur,
				SourceRowCount:      a.n,
			})
			seen[cat] = true
		}
	}
	for cat, a := range m {
		if seen[cat] || a == nil {
			continue
		}
		rows = append(rows, &pbbilling.BillingCategoryRow{
			Provider:            pbtenant.CloudProvider_huawei,
			AccountName:         tenant.AccountName(),
			BillingCycle:        billingCycle,
			Category:            cat,
			TotalConsumeAmount:  billingagg.RoundMoney2(a.sum),
			Currency:            cur,
			SourceRowCount:      a.n,
		})
	}

	return &pbbilling.ListBillingSummaryResp{
		Rows:              rows,
		GrandTotalConsume: billingagg.RoundMoney2(grand),
		Currency:          cur,
	}, nil
}
