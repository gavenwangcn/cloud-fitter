package wafbind

import (
	"strings"

	"github.com/golang/glog"

	"github.com/cloud-fitter/cloud-fitter/internal/server/cert"
	"github.com/cloud-fitter/cloud-fitter/internal/server/waf"
)

func wafAccountSet(wafAccountNames []string) map[string]struct{} {
	set := make(map[string]struct{}, len(wafAccountNames))
	for _, a := range wafAccountNames {
		a = strings.TrimSpace(a)
		if a != "" {
			set[a] = struct{}{}
		}
	}
	return set
}

// FilterWAFRows 仅保留 CLOUD_FITTER_HUAWEI_WAF_CMDB_ACCOUNT_NAMES 指定账号下的 WAF 记录（读/写镜像与 CMDB 快照同步共用）。
func FilterWAFRows(rows []*waf.Instance, wafAccountNames []string) []*waf.Instance {
	allowed := wafAccountSet(wafAccountNames)
	if len(allowed) == 0 {
		return nil
	}
	out := make([]*waf.Instance, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		if _, ok := allowed[row.AccountName]; !ok {
			continue
		}
		out = append(out, row)
	}
	return out
}

// FilterCertRows 仅保留指定 WAF/证书账号下的 SCM 证书记录（读/写镜像与 CMDB 快照同步共用）。
func FilterCertRows(rows []*cert.Instance, wafAccountNames []string) []*cert.Instance {
	allowed := wafAccountSet(wafAccountNames)
	if len(allowed) == 0 {
		return nil
	}
	out := make([]*cert.Instance, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		if _, ok := allowed[row.AccountName]; !ok {
			continue
		}
		out = append(out, row)
	}
	return out
}

// LogWAFSnapshotFilter 镜像读库后 WAF 按账号过滤统计。
func LogWAFSnapshotFilter(systemID string, wafAccounts []string, raw, kept int) {
	glog.Infof("resourcecache waf snap(filter): system_id=%s waf_accounts=%v rows raw=%d kept=%d",
		systemID, wafAccounts, raw, kept)
}

// LogCertSnapshotFilter 镜像读库后证书按账号过滤统计。
func LogCertSnapshotFilter(systemID string, wafAccounts []string, raw, kept int) {
	glog.Infof("resourcecache cert snap(filter): system_id=%s waf_accounts=%v rows raw=%d kept=%d",
		systemID, wafAccounts, raw, kept)
}
