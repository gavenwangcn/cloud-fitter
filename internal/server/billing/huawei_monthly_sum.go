package billing

import (
	"context"
	"strings"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
	bssv2 "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bss/v2"
	bssmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bss/v2/model"
	bssregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bss/v2/region"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/internal/huaweicloudregion"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

// HuaweiBillSumLine 华为云 BSS ShowCustomerMonthlySum 单行。
type HuaweiBillSumLine struct {
	ServiceTypeCode string
	ConsumeAmount   float64
}

func huaweiFetchMonthlySumLines(ctx context.Context, tenant tenanter.Tenanter, billingCycle string) (lines []HuaweiBillSumLine, currency string, err error) {
	defer func() {
		if r := recover(); r != nil {
			lines = nil
			err = errors.Errorf("huawei billing: sdk panic (often DNS/network to IAM): %v", r)
		}
	}()

	t, ok := tenant.(*tenanter.AccessKeyTenant)
	if !ok {
		return nil, "", errors.New("huawei billing: only AccessKeyTenant supported")
	}
	auth := global.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).Build()
	hc := bssv2.BssClientBuilder().WithRegion(bssregion.ValueOf("cn-north-1")).WithCredential(auth).WithHttpConfig(huaweicloudregion.SDKHttpConfig()).Build()
	cli := bssv2.NewBssClient(hc)

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
			return nil, "", errors.Wrap(err, "ShowCustomerMonthlySum")
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
			amt := 0.0
			if row.ConsumeAmount != nil {
				amt = row.ConsumeAmount.InexactFloat64()
			}
			lines = append(lines, HuaweiBillSumLine{
				ServiceTypeCode: svc,
				ConsumeAmount:   amt,
			})
		}
		batch := int32(len(*resp.BillSums))
		if resp.TotalCount == nil || batch == 0 || offset+batch >= *resp.TotalCount {
			break
		}
		offset += batch
	}

	if strings.TrimSpace(currency) == "" {
		currency = "CNY"
	}
	return lines, strings.TrimSpace(currency), nil
}
