package billing

import (
	"context"
	"strings"

	"github.com/golang/glog"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
	bssv2 "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bss/v2"
	bssmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bss/v2/model"
	bssregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bss/v2/region"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbbilling"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/billingagg"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweicloudregion"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

func huaweiListBillingSummary(ctx context.Context, tenant tenanter.Tenanter, billingCycle string) (resp *pbbilling.ListBillingSummaryResp, err error) {
	defer func() {
		if r := recover(); r != nil {
			resp = nil
			err = errors.Errorf("huawei billing: sdk panic (often DNS/network to IAM): %v", r)
		}
	}()

	t, ok := tenant.(*tenanter.AccessKeyTenant)
	if !ok {
		return nil, errors.New("huawei billing: only AccessKeyTenant supported")
	}
	// BSS 属于华为云 global 服务，需使用 global.Credentials（官方 SDK 文档）。
	auth := global.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).Build()
	hc := bssv2.BssClientBuilder().WithRegion(bssregion.ValueOf("cn-north-1")).WithCredential(auth).WithHttpConfig(huaweicloudregion.SDKHttpConfig()).Build()
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
				amt = row.ConsumeAmount.InexactFloat64()
			}
			if cat == "其他" {
				glog.Infof("huawei billing ShowCustomerMonthlySum -> 其他: account=%s bill_cycle=%s service_type_code=%q consume_amount=%.2f currency=%s",
					tenant.AccountName(), billingCycle, svc, amt, currencyOrDash(resp.Currency))
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
	seen := make(map[string]bool)
	for _, cat := range billingagg.BillingCategoryDisplayOrder {
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

func currencyOrDash(p *string) string {
	if p != nil && strings.TrimSpace(*p) != "" {
		return strings.TrimSpace(*p)
	}
	return "-"
}
