package wafbind

import (
	"os"
	"strconv"
	"strings"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/server/eip"
	"github.com/google/uuid"
	"github.com/cloud-fitter/cloud-fitter/internal/server/waf"
)

// EIPDomainRow EIP 与 WAF 匹配后的域名绑定（供快照与 CMDB）。
type EIPDomainRow struct {
	EIPResourceKey string   `json:"eipResourceKey"`
	SysNodeKey     string   `json:"sysNodeKey"`
	Domains        []string `json:"domains"`
}

// NodeDomainRow 节点维度域名绑定。
type NodeDomainRow struct {
	SysNodeKey string   `json:"sysNodeKey"`
	Domains    []string `json:"domains"`
}

// CertDomainJob 证书名 + 绑定域名（按账号）。
type CertDomainJob struct {
	AccountName string   `json:"accountName"`
	CertName    string   `json:"certName"`
	Domains     []string `json:"domains"`
}

// Result WAF 与 EIP 匹配结果。
type Result struct {
	EIPDomains  []EIPDomainRow
	NodeDomains []NodeDomainRow
	CertJobs    []CertDomainJob
}

// AccountNamesFromEnv 读取 CLOUD_FITTER_HUAWEI_WAF_CMDB_ACCOUNT_NAMES（逗号分隔）。
// 仅在这些账号上拉 WAF/证书；与 CMDB 系统关联账号无关，匹配时用 EIP 公网 IP（e.Eip / CMDB eip_ip）。
func AccountNamesFromEnv() []string {
	raw := strings.TrimSpace(os.Getenv("CLOUD_FITTER_HUAWEI_WAF_CMDB_ACCOUNT_NAMES"))
	if raw == "" {
		return nil
	}
	var out []string
	seen := make(map[string]struct{})
	for _, p := range strings.Split(raw, ",") {
		n := strings.TrimSpace(p)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}

// IntersectAccountNames 配置账号与系统关联账号的交集。
func IntersectAccountNames(configured, linked []string) []string {
	linkedSet := make(map[string]struct{}, len(linked))
	for _, n := range linked {
		if n = strings.TrimSpace(n); n != "" {
			linkedSet[n] = struct{}{}
		}
	}
	var out []string
	for _, n := range configured {
		if _, ok := linkedSet[n]; ok {
			out = append(out, n)
		}
	}
	return out
}

// Build 将 WAF 防护域名与证书绑定到本系统 EIP/节点：
//   - wafRows 仅保留 wafAccountNames 指定账号下的记录；
//   - 用 WAF 源站 IP/端口（OriginServers，解析为 IP）与 EIP 弹性公网 IP（e.Eip，对应 CMDB eip_ip）相等则匹配；
//   - 不要求 WAF 账号与 EIP 所属云账号同名（跨账号 Infra WAF + 业务账号 EIP）。
func Build(eips []*eip.Instance, wafRows []*waf.Instance, wafAccountNames []string) Result {
	allowed := make(map[string]struct{}, len(wafAccountNames))
	for _, a := range wafAccountNames {
		allowed[a] = struct{}{}
	}
	byEIP := make(map[string]*EIPDomainRow)
	byNode := make(map[string][]string)
	certToDomains := make(map[string][]string)

	for _, row := range wafRows {
		if row == nil || strings.TrimSpace(row.Hostname) == "" {
			continue
		}
		if _, ok := allowed[row.AccountName]; len(allowed) > 0 && !ok {
			continue
		}
		domain := strings.TrimSpace(row.Hostname)
		originIPs := ParseOriginIPs(row.OriginServers)
		if len(originIPs) == 0 {
			continue
		}
		for _, e := range eips {
			if e == nil {
				continue
			}
			// 跨账号：WAF 常在 Infra 等共享账号，EIP 在业务账号；按源站公网 IP 与 EIP 匹配。
			pub := strings.TrimSpace(e.Eip)
			if pub == "" || !sliceContains(originIPs, pub) {
				continue
			}
			eipKey := EIPResourceKey(e.EipId)
			if eipKey == "" {
				continue
			}
			sysNode := SysNodeKeyFromEIP(e)
			if sysNode == "" {
				continue
			}
			r, ok := byEIP[eipKey]
			if !ok {
				r = &EIPDomainRow{EIPResourceKey: eipKey, SysNodeKey: sysNode}
				byEIP[eipKey] = r
			}
			r.Domains = appendUnique(r.Domains, domain)
			byNode[sysNode] = appendUnique(byNode[sysNode], domain)
			if certName := strings.TrimSpace(row.CertificateName); certName != "" {
				ck := row.AccountName + "|" + certName
				certToDomains[ck] = appendUnique(certToDomains[ck], domain)
			}
		}
	}

	var res Result
	for _, r := range byEIP {
		if len(r.Domains) > 0 {
			res.EIPDomains = append(res.EIPDomains, *r)
		}
	}
	for node, domains := range byNode {
		if len(domains) > 0 {
			res.NodeDomains = append(res.NodeDomains, NodeDomainRow{SysNodeKey: node, Domains: domains})
		}
	}
	for ck, domains := range certToDomains {
		parts := strings.SplitN(ck, "|", 2)
		if len(parts) != 2 {
			continue
		}
		res.CertJobs = append(res.CertJobs, CertDomainJob{
			AccountName: parts[0],
			CertName:    parts[1],
			Domains:     domains,
		})
	}
	return res
}

// ParseOriginIP 从 ip:port 解析 IP。
func ParseOriginIP(hostPort string) string {
	hostPort = strings.TrimSpace(hostPort)
	if hostPort == "" {
		return ""
	}
	if i := strings.LastIndex(hostPort, ":"); i > 0 {
		if _, err := strconv.Atoi(hostPort[i+1:]); err == nil {
			return strings.TrimSpace(hostPort[:i])
		}
	}
	return hostPort
}

// ParseOriginIPs 解析逗号分隔的源站列表。
func ParseOriginIPs(originServers string) []string {
	var ips []string
	seen := make(map[string]struct{})
	for _, part := range strings.Split(originServers, ",") {
		ip := ParseOriginIP(strings.TrimSpace(part))
		if ip == "" {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		ips = append(ips, ip)
	}
	return ips
}

func sliceContains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func appendUnique(base []string, add ...string) []string {
	seen := make(map[string]struct{}, len(base)+len(add))
	for _, s := range base {
		s = strings.TrimSpace(s)
		if s != "" {
			seen[s] = struct{}{}
		}
	}
	for _, s := range add {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		base = append(base, s)
	}
	return base
}

func eipTenantProvider(e *eip.Instance) pbtenant.CloudProvider {
	switch strings.ToLower(strings.TrimSpace(e.Provider)) {
	case "ali", "aliyun":
		return pbtenant.CloudProvider_ali
	case "tencent":
		return pbtenant.CloudProvider_tencent
	case "aws":
		return pbtenant.CloudProvider_aws
	default:
		return pbtenant.CloudProvider_huawei
	}
}

func cloudTypeLabel(p pbtenant.CloudProvider) string {
	switch p {
	case pbtenant.CloudProvider_huawei:
		return "华为云"
	case pbtenant.CloudProvider_ali:
		return "阿里云"
	case pbtenant.CloudProvider_tencent:
		return "腾讯云"
	case pbtenant.CloudProvider_aws:
		return "AWS"
	default:
		return "云"
	}
}

// SysNodeKeyFromEIP 从 EIP 实例计算 CMDB 节点名（与 Build 内逻辑一致）。
func SysNodeKeyFromEIP(e *eip.Instance) string {
	if e == nil {
		return ""
	}
	return SysNodeKey(eipTenantProvider(e), strings.TrimSpace(e.RegionName), e.NodeTagValue)
}

// SysNodeKey 与 CMDB effectiveSysNodeName 一致。
func SysNodeKey(p pbtenant.CloudProvider, region, nodeTag string) string {
	baseCloud := cloudTypeLabel(p)
	region = strings.TrimSpace(region)
	s := strings.TrimSpace(nodeTag)
	base := ""
	if region != "" {
		base = baseCloud + "-" + region
	} else if baseCloud != "" {
		base = baseCloud
	}
	if s != "" && base != "" && strings.HasPrefix(s, base) {
		return s
	}
	if s != "" && base != "" {
		return base + "-" + s
	}
	if s != "" {
		return s
	}
	return base
}

// EIPResourceKey 快照/CMDB 用的 EIP 主键（与 cloud_snap_eip.resource_key 一致）。
func EIPResourceKey(eipID string) string {
	eipID = strings.TrimSpace(eipID)
	if eipID == "" {
		return ""
	}
	if u, err := uuid.Parse(eipID); err == nil {
		return u.String()
	}
	if len(eipID) > 128 {
		return eipID[:128]
	}
	return eipID
}
