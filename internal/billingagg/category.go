package billingagg

import (
	"strings"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbbilling"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
)

// CategoryFromBillLine 根据阿里云/腾讯云账单明细行的产品字段归到大类。
func CategoryFromBillLine(provider pbtenant.CloudProvider, productCode, productType string) string {
	pc := strings.ToLower(strings.TrimSpace(productCode))
	pt := strings.ToLower(strings.TrimSpace(productType))
	switch provider {
	case pbtenant.CloudProvider_ali:
		if strings.Contains(pc, "ecs") || strings.Contains(pc, "ecm") || pt == "云服务器 ecs" {
			return "ECS"
		}
		if strings.Contains(pc, "rds") || strings.Contains(pt, "关系型数据库") {
			return "RDS"
		}
		if strings.Contains(pc, "kvstore") || strings.Contains(pc, "redis") || strings.Contains(pt, "redis") {
			return "DCS"
		}
		if strings.Contains(pc, "kafka") || strings.Contains(pt, "kafka") {
			return "DMS"
		}
		if strings.Contains(pc, "mongodb") || strings.Contains(pc, "dds") {
			return "MongoDB"
		}
		if strings.Contains(pc, "cs") || strings.Contains(pc, "k8s") || strings.Contains(pt, "容器") {
			return "CCE"
		}
	case pbtenant.CloudProvider_tencent:
		if strings.HasPrefix(pc, "sp_cvm") || strings.Contains(pc, "cvm") {
			return "ECS"
		}
		if strings.HasPrefix(pc, "sp_cdb") || strings.Contains(pc, "cdb") || strings.Contains(pc, "postgres") {
			return "RDS"
		}
		if strings.HasPrefix(pc, "sp_redis") || strings.Contains(pc, "redis") {
			return "DCS"
		}
		if strings.Contains(pc, "ckafka") || strings.Contains(pc, "kafka") {
			return "DMS"
		}
		if strings.Contains(pc, "tke") || strings.Contains(pc, "eks") {
			return "CCE"
		}
	}
	return "其他"
}

// HuaweiCategoryFromServiceType 华为云 BSS 汇总账单中的云服务类型编码 -> 展示大类。
// 参考账单字段说明：ECS 为 hws.service.type.ec2 等。
func HuaweiCategoryFromServiceType(serviceTypeCode string) string {
	switch strings.TrimSpace(serviceTypeCode) {
	case "hws.service.type.ec2":
		return "ECS"
	case "hws.service.type.rds":
		return "RDS"
	case "hws.service.type.dcs":
		return "DCS"
	case "hws.service.type.dms":
		return "DMS"
	case "hws.service.type.cce":
		return "CCE"
	default:
		return "其他"
	}
}

// RoundMoney2 金额保留两位小数（展示用）。
func RoundMoney2(x float64) float64 {
	return float64(int64(x*100+0.5)) / 100
}

// AggregateBillInstances 将明细行按大类汇总。
func AggregateBillInstances(
	provider pbtenant.CloudProvider,
	accountName, billingCycle, currency string,
	billings []*pbbilling.BillingInstance,
) *pbbilling.ListBillingSummaryResp {
	type agg struct {
		sum float64
		n   int32
	}
	m := make(map[string]*agg)
	var grand float64
	for _, b := range billings {
		if b == nil {
			continue
		}
		cat := CategoryFromBillLine(provider, b.ProductCode, b.ProductType)
		a := m[cat]
		if a == nil {
			a = &agg{}
			m[cat] = a
		}
		a.sum += b.Fee
		a.n++
		grand += b.Fee
	}
	cur := currency
	if cur == "" {
		cur = "CNY"
	}
	rows := make([]*pbbilling.BillingCategoryRow, 0, len(m))
	order := []string{"ECS", "RDS", "DCS", "DMS", "CCE", "MongoDB", "其他"}
	seen := make(map[string]bool)
	for _, cat := range order {
		if a, ok := m[cat]; ok && a != nil {
			rows = append(rows, &pbbilling.BillingCategoryRow{
				Provider:            provider,
				AccountName:         accountName,
				BillingCycle:        billingCycle,
				Category:            cat,
				TotalConsumeAmount:  RoundMoney2(a.sum),
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
			Provider:            provider,
			AccountName:         accountName,
			BillingCycle:        billingCycle,
			Category:            cat,
			TotalConsumeAmount:  RoundMoney2(a.sum),
			Currency:            cur,
			SourceRowCount:      a.n,
		})
	}
	return &pbbilling.ListBillingSummaryResp{
		Rows:               rows,
		GrandTotalConsume:  RoundMoney2(grand),
		Currency:           cur,
	}
}
