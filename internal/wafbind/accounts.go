package wafbind

import (
	"strings"

	"github.com/golang/glog"
)

// WAFAccountsForPull 返回用于调用华为 WAF/SCM 证书 API 的云账号名列表。
// 仅来自 CLOUD_FITTER_HUAWEI_WAF_CMDB_ACCOUNT_NAMES；WAF 与证书均在此账号拉取，不用系统关联账号。
// 系统 EIP 仍从关联账号拉取，通过 WAF 源站公网 IP 与 EIP 公网 IP（eip_ip）跨账号匹配。
func WAFAccountsForPull(configured []string) []string {
	return configured
}

// LogCMDBWAFContext 在 CMDB WAF/证书派生同步开始时输出账号与数据源上下文。
func LogCMDBWAFContext(systemID string, configured, linked, wafPull []string, eipCount int, fromSnapshot bool) {
	mode := "api"
	if fromSnapshot {
		mode = "snapshot"
	}
	overlap := IntersectAccountNames(configured, linked)
	glog.Infof("cmdb sync waf(begin): system_id=%s mode=%s env_accounts=%v linked_accounts=%v waf_pull_accounts=%v name_overlap=%v eips=%d",
		systemID, mode, configured, linked, wafPull, overlap, eipCount)
	if len(wafPull) == 0 {
		glog.Warningf("cmdb sync waf(skip): system_id=%s CLOUD_FITTER_HUAWEI_WAF_CMDB_ACCOUNT_NAMES is empty", systemID)
		return
	}
	if len(linked) == 0 {
		glog.Warningf("cmdb sync waf(hint): system_id=%s no linked cloud accounts; EIP list empty, domain match unlikely", systemID)
	} else if len(overlap) == 0 {
		glog.Infof("cmdb sync waf(cross-account): system_id=%s WAF/cert from %v, system resources from %v (match WAF origin public IP to EIP eip field)",
			systemID, wafPull, linked)
	}
}

// LogBuildResult 输出 WAF 与 EIP 匹配绑定结果；无绑定时给出排查提示。
func LogBuildResult(systemID string, wafRows, eipCount int, bind Result) {
	glog.Infof("cmdb sync waf(bind): system_id=%s waf_rows=%d eips=%d eip_bindings=%d node_bindings=%d cert_jobs=%d",
		systemID, wafRows, eipCount, len(bind.EIPDomains), len(bind.NodeDomains), len(bind.CertJobs))
	if wafRows > 0 && eipCount > 0 && len(bind.EIPDomains) == 0 && len(bind.NodeDomains) == 0 {
		glog.Warningf("cmdb sync waf(no_match): system_id=%s waf_rows=%d eips=%d but zero domain bindings; verify WAF origin IP equals EIP public IP",
			systemID, wafRows, eipCount)
	}
}

// LogSnapshotWAFContext 资源快照拉取 WAF/证书前的账号上下文。
func LogSnapshotWAFContext(systemID, systemName string, configured, linked, wafPull []string, eipCount int) {
	overlap := IntersectAccountNames(configured, linked)
	glog.Infof("resource snapshot: waf(begin) system_id=%s system_name=%q env_accounts=%v linked_accounts=%v waf_pull_accounts=%v name_overlap=%v eips=%d",
		systemID, systemName, configured, linked, wafPull, overlap, eipCount)
	if len(wafPull) == 0 {
		return
	}
	if len(overlap) == 0 && len(linked) > 0 {
		glog.Infof("resource snapshot: waf(cross-account) system_id=%s WAF from %v, EIP from linked %v",
			systemID, wafPull, linked)
	}
}

// SampleEIPPublicIPs 取前 n 个 EIP 公网 IP 用于日志（脱敏展示）。
func SampleEIPPublicIPs(eips []string, n int) []string {
	if n <= 0 {
		n = 5
	}
	var out []string
	for _, ip := range eips {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		out = append(out, ip)
		if len(out) >= n {
			break
		}
	}
	return out
}
