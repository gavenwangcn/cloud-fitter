package billing

import (
	"context"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbbilling"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/billingagg"
	"github.com/cloud-fitter/cloud-fitter/internal/service/billinger"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

// ListSummary 按账号与账期返回 ECS/RDS/DCS 等大类的消费汇总。
func ListSummary(ctx context.Context, req *pbbilling.ListBillingSummaryReq) (*pbbilling.ListBillingSummaryResp, error) {
	if req == nil {
		return nil, errors.New("nil request")
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.UTC
	}
	cycle := strings.TrimSpace(req.BillingCycle)
	if cycle == "" {
		cycle = time.Now().In(loc).Format("2006-01")
	}

	tenanters, err := tenanter.GetTenanters(req.Provider)
	if err != nil {
		return nil, errors.WithMessage(err, "getTenanters")
	}
	for _, tenant := range tenanters {
		if req.AccountName == "" || tenant.AccountName() == req.AccountName {
			return summaryForTenant(ctx, req.Provider, tenant, cycle)
		}
	}
	return nil, errors.New("account not found or no matching tenant")
}

func summaryForTenant(ctx context.Context, provider pbtenant.CloudProvider, tenant tenanter.Tenanter, billingCycle string) (*pbbilling.ListBillingSummaryResp, error) {
	switch provider {
	case pbtenant.CloudProvider_huawei:
		return huaweiListBillingSummary(ctx, tenant, billingCycle)
	case pbtenant.CloudProvider_ali, pbtenant.CloudProvider_tencent:
		cli, err := billinger.NewBillingClient(provider, tenant)
		if err != nil {
			return nil, errors.WithMessage(err, "NewBillingClient")
		}
		detail, err := cli.ListDetail(ctx, &pbbilling.ListDetailReq{
			Provider:     provider,
			BillingCycle: billingCycle,
			AccountName:  tenant.AccountName(),
		})
		if err != nil {
			return nil, err
		}
		return billingagg.AggregateBillInstances(provider, tenant.AccountName(), billingCycle, "CNY", detail.Billings), nil
	default:
		return nil, errors.Errorf("billing summary not supported for provider %v", provider)
	}
}
