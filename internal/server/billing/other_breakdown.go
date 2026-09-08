package billing

import (
	"context"
	"sort"
	"strings"

	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/billingagg"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

// OtherBreakdownRow 「其他」弹框明细行。
type OtherBreakdownRow struct {
	AccountName     string  `json:"accountName"`
	Category        string  `json:"category"`
	ServiceTypeCode string  `json:"serviceTypeCode"`
	ConsumeAmount   float64 `json:"consumeAmount"`
	Currency        string  `json:"currency"`
}

// OtherBreakdownResult 「其他」弹框明细响应。
type OtherBreakdownResult struct {
	Rows     []OtherBreakdownRow `json:"rows"`
	Total    float64             `json:"total"`
	Currency string              `json:"currency"`
}

// OtherBreakdownAccount 待查询的云账号。
type OtherBreakdownAccount struct {
	Provider    pbtenant.CloudProvider
	AccountName string
}

// ListOtherBreakdown 返回主表「其他」行下的华为云费用明细（按展示大类拆分）。
func ListOtherBreakdown(
	ctx context.Context,
	provider pbtenant.CloudProvider,
	billingCycle, accountName, systemName string,
	resolveSystemAccounts func(systemName string) ([]OtherBreakdownAccount, error),
) (*OtherBreakdownResult, error) {
	if provider != pbtenant.CloudProvider_huawei {
		return nil, errors.New("other breakdown only supported for huawei cloud")
	}
	cycle := strings.TrimSpace(billingCycle)
	if cycle == "" {
		return nil, errors.New("billingMonth is required")
	}

	var targets []OtherBreakdownAccount
	systemName = strings.TrimSpace(systemName)
	accountName = strings.TrimSpace(accountName)
	switch {
	case systemName != "":
		if resolveSystemAccounts == nil {
			return nil, errors.New("system account resolver is required")
		}
		accs, err := resolveSystemAccounts(systemName)
		if err != nil {
			return nil, err
		}
		targets = accs
	case accountName != "":
		targets = []OtherBreakdownAccount{{Provider: provider, AccountName: accountName}}
	default:
		return nil, errors.New("accountName or systemName is required")
	}
	if len(targets) == 0 {
		return &OtherBreakdownResult{Currency: "CNY"}, nil
	}

	out := make([]OtherBreakdownRow, 0)
	currency := "CNY"
	var total float64
	for _, acc := range targets {
		if acc.Provider != pbtenant.CloudProvider_huawei {
			continue
		}
		tenant, err := findTenant(acc.Provider, acc.AccountName)
		if err != nil {
			return nil, err
		}
		lines, cur, err := huaweiFetchMonthlySumLines(ctx, tenant, cycle)
		if err != nil {
			return nil, errors.Wrapf(err, "huawei other breakdown account=%q cycle=%q", acc.AccountName, cycle)
		}
		if cur != "" {
			currency = cur
		}
		for _, line := range lines {
			if !billingagg.HuaweiIsOtherSummaryCategory(line.ServiceTypeCode) {
				continue
			}
			if line.ConsumeAmount == 0 {
				continue
			}
			amt := billingagg.RoundMoney2(line.ConsumeAmount)
			out = append(out, OtherBreakdownRow{
				AccountName:     acc.AccountName,
				Category:        billingagg.HuaweiOtherDetailCategory(line.ServiceTypeCode),
				ServiceTypeCode: line.ServiceTypeCode,
				ConsumeAmount:   amt,
				Currency:        cur,
			})
			total += amt
		}
	}

	sortOtherBreakdownRows(out)
	return &OtherBreakdownResult{
		Rows:     out,
		Total:    billingagg.RoundMoney2(total),
		Currency: currency,
	}, nil
}

func findTenant(provider pbtenant.CloudProvider, accountName string) (tenanter.Tenanter, error) {
	tenanters, err := tenanter.GetTenanters(provider)
	if err != nil {
		return nil, errors.WithMessage(err, "getTenanters")
	}
	for _, t := range tenanters {
		if t.AccountName() == accountName {
			return t, nil
		}
	}
	return nil, errors.Errorf("account not found: %q", accountName)
}

func sortOtherBreakdownRows(rows []OtherBreakdownRow) {
	catOrder := make(map[string]int, len(billingagg.HuaweiOtherDetailCategoryOrder))
	for i, c := range billingagg.HuaweiOtherDetailCategoryOrder {
		catOrder[c] = i
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.AccountName != b.AccountName {
			return a.AccountName < b.AccountName
		}
		oa, ob := catOrder[a.Category], catOrder[b.Category]
		if oa != ob {
			return oa < ob
		}
		if a.ServiceTypeCode != b.ServiceTypeCode {
			return a.ServiceTypeCode < b.ServiceTypeCode
		}
		return a.ConsumeAmount > b.ConsumeAmount
	})
}
