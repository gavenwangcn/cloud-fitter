package billingagg

import (
	"strings"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbbilling"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
)

// BillingCategoryDisplayOrder 账单汇总表大类列顺序（有金额的类别才出现在结果中）。
var BillingCategoryDisplayOrder = []string{
	"ECS", "RDS", "DCS", "DMS", "CCE", "MongoDB",
	"EIP/网络", "负载均衡", "对象存储", "VPC",
	"云硬盘", "云防火墙", "主机安全", "NAT网关", "日志服务", "云监控", "云商店",
	"其他",
}

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
		if strings.Contains(pc, "oss") || strings.Contains(pt, "对象存储") {
			return "对象存储"
		}
		if strings.Contains(pc, "slb") || strings.Contains(pc, "alb") || strings.Contains(pc, "nlb") ||
			strings.Contains(pt, "负载均衡") || strings.Contains(pt, "应用型负载均衡") {
			return "负载均衡"
		}
		if strings.Contains(pc, "eip") || strings.Contains(pc, "cbwp") || strings.Contains(pc, "bandwidthpackage") ||
			strings.Contains(pt, "弹性公网") || strings.Contains(pt, "共享带宽") || strings.Contains(pc, "vpn") {
			return "EIP/网络"
		}
		if strings.Contains(pc, "vpc") || strings.Contains(pc, "nat_gateway") || strings.Contains(pc, "natgateway") ||
			strings.Contains(pt, "专有网络") || strings.Contains(pt, "nat网关") {
			return "VPC"
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
		if strings.Contains(pc, "mongodb") {
			return "MongoDB"
		}
		if strings.Contains(pc, "tke") || strings.Contains(pc, "eks") {
			return "CCE"
		}
		if strings.Contains(pc, "cos") || strings.HasPrefix(pc, "sp_cos") || strings.Contains(pt, "对象存储") {
			return "对象存储"
		}
		if strings.Contains(pc, "clb") || strings.Contains(pc, "elb") || strings.Contains(pc, "alb") ||
			strings.HasPrefix(pc, "sp_clb") || strings.Contains(pt, "负载均衡") {
			return "负载均衡"
		}
		if strings.Contains(pc, "eip") || strings.Contains(pc, "bwp") || strings.Contains(pc, "anycasteip") ||
			strings.Contains(pt, "弹性公网") || strings.Contains(pt, "共享带宽") || strings.Contains(pc, "vpn") {
			return "EIP/网络"
		}
		if strings.Contains(pc, "vpc") || strings.Contains(pc, "nat") || strings.Contains(pt, "私有网络") ||
			strings.Contains(pt, "nat网关") {
			return "VPC"
		}
	}
	return "其他"
}

// HuaweiCategoryFromServiceType 华为云 BSS 汇总账单中的云服务类型编码 -> 展示大类。
// 编码参见费用中心/ShowCustomerMonthlySum 等接口返回的 service_type_code（未覆盖的类型归入「其他」）。
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
	case "hws.service.type.obs":
		return "对象存储"
	case "hws.service.type.vpc":
		return "VPC"
	case "hws.service.type.eip":
		return "EIP/网络"
	case "hws.service.type.elb", "hws.service.type.loadbalancer":
		return "负载均衡"
	// 块存储（账单侧常见编码为 ebs；evs 亦归入云硬盘）
	case "hws.service.type.ebs", "hws.service.type.evs":
		return "云硬盘"
	case "hws.service.type.cfw":
		return "云防火墙"
	case "hws.service.type.hss":
		return "主机安全"
	case "hws.service.type.marketplace":
		return "云商店"
	case "hws.service.type.lts":
		return "日志服务"
	case "hws.service.type.ces":
		return "云监控"
	// NAT 网关（与专有网络 VPC 分列）；BSS 可能返回 nat_gateway / natgw / natgateway
	case "hws.service.type.nat_gateway", "hws.service.type.natgw", "hws.service.type.natgateway":
		return "NAT网关"
	case "hws.service.type.vpn":
		return "EIP/网络"
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
	seen := make(map[string]bool)
	for _, cat := range BillingCategoryDisplayOrder {
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
