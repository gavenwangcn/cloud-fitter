package cmdb

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/configstore"
	"github.com/cloud-fitter/cloud-fitter/internal/server/cert"
	"github.com/cloud-fitter/cloud-fitter/internal/server/eip"
	"github.com/cloud-fitter/cloud-fitter/internal/server/scope"
	"github.com/cloud-fitter/cloud-fitter/internal/server/waf"
	"github.com/cloud-fitter/cloud-fitter/internal/wafbind"
)

// syncWAFDerivedCMDB 从 CLOUD_FITTER_HUAWEI_WAF_CMDB_ACCOUNT_NAMES 指定账号拉 WAF/证书（不用系统关联账号），
// 再按 WAF 源站公网 IP 与本系统 EIP 的 e.Eip（CMDB eip_ip）匹配，回写 domain_name 并同步证书 CI。
func (s *Syncer) syncWAFDerivedCMDB(ctx context.Context, systemID string, eips []*eip.Instance, linkedAccounts []configstore.Row, wafAccountNames []string) (domainSt, certStats componentSyncStats) {
	linked := accountNamesFromRows(linkedAccounts)
	wafPull := wafbind.WAFAccountsForPull(wafAccountNames)
	wafbind.LogCMDBWAFContext(systemID, wafAccountNames, linked, wafPull, len(eips), false)
	if len(wafPull) == 0 {
		return domainSt, certStats
	}

	var allWaf []*waf.Instance
	for _, accName := range wafPull {
		accCtx := scope.WithAccountName(ctx, accName)
		wafRows, err := waf.List(accCtx, pbtenant.CloudProvider_huawei)
		if err != nil {
			glog.Warningf("cmdb sync waf(list): system_id=%s account=%s err=%v", systemID, accName, err)
			domainSt.Errors++
			continue
		}
		glog.Infof("cmdb sync waf(list ok): system_id=%s account=%s rows=%d", systemID, accName, len(wafRows))
		allWaf = append(allWaf, wafRows...)
	}
	bind := wafbind.Build(eips, allWaf, wafPull)
	wafbind.LogBuildResult(systemID, len(allWaf), len(eips), bind)
	if len(eips) > 0 {
		var eipIPs []string
		for _, e := range eips {
			if e != nil {
				eipIPs = append(eipIPs, e.Eip)
			}
		}
		glog.Infof("cmdb sync waf(eip_sample): system_id=%s public_ips=%v", systemID, wafbind.SampleEIPPublicIPs(eipIPs, 8))
	}

	for _, r := range bind.EIPDomains {
		st := s.patchCMDBCIDomainName("_type:EIP", fmt.Sprintf("uuid:%s,system_id:%s", r.EIPResourceKey, systemID), systemID, "eip", r.EIPResourceKey, r.Domains)
		domainSt.Added += st.Added
		domainSt.Updated += st.Updated
		domainSt.Skipped += st.Skipped
		domainSt.Errors += st.Errors
	}
	for _, r := range bind.NodeDomains {
		st := s.patchCMDBCIDomainName("_type:system_node", fmt.Sprintf("sys_node_name:%s,system_id:%s", r.SysNodeKey, systemID), systemID, "system_node", r.SysNodeKey, r.Domains)
		domainSt.Added += st.Added
		domainSt.Updated += st.Updated
		domainSt.Skipped += st.Skipped
		domainSt.Errors += st.Errors
	}

	certIndexByAccount := loadCertIndexFromWAFAccounts(ctx, systemID, wafPull)
	certStats = s.upsertCertificatesFromJobs(ctx, systemID, bind.CertJobs, certIndexByAccount)

	glog.Infof("cmdb sync waf(done): system_id=%s eip_domain_bindings=%d node_bindings=%d domain(add=%d,upd=%d,skip=%d,err=%d) cert(add=%d,upd=%d,skip=%d,err=%d)",
		systemID, len(bind.EIPDomains), len(bind.NodeDomains),
		domainSt.Added, domainSt.Updated, domainSt.Skipped, domainSt.Errors,
		certStats.Added, certStats.Updated, certStats.Skipped, certStats.Errors)
	return domainSt, certStats
}

// loadCertIndexFromWAFAccounts 从 CLOUD_FITTER_HUAWEI_WAF_CMDB_ACCOUNT_NAMES 指定账号拉 SCM 全量证书（不用系统关联账号）。
func loadCertIndexFromWAFAccounts(ctx context.Context, systemID string, wafPull []string) map[string]map[string]*cert.Instance {
	out := make(map[string]map[string]*cert.Instance, len(wafPull))
	for _, accName := range wafPull {
		accCtx := scope.WithAccountName(ctx, accName)
		certRows, err := cert.List(accCtx, pbtenant.CloudProvider_huawei)
		if err != nil {
			glog.Warningf("cmdb sync certificate(list): system_id=%s account=%s err=%v", systemID, accName, err)
			continue
		}
		glog.Infof("cmdb sync certificate(list ok): system_id=%s account=%s rows=%d (waf env account, not system linked)",
			systemID, accName, len(certRows))
		out[accName] = indexCertificatesByName(certRows)
	}
	return out
}

// certIndexFromSnapshot 将已按 WAF 账号过滤后的镜像证书按账号建索引（LoadCertificates 已过滤，此处幂等）。
func certIndexFromSnapshot(certRows []*cert.Instance, wafPull []string) map[string]map[string]*cert.Instance {
	filtered := wafbind.FilterCertRows(certRows, wafPull)
	byAccount := make(map[string][]*cert.Instance)
	for _, c := range filtered {
		if c == nil {
			continue
		}
		byAccount[c.AccountName] = append(byAccount[c.AccountName], c)
	}
	out := make(map[string]map[string]*cert.Instance, len(byAccount))
	for acc, rows := range byAccount {
		out[acc] = indexCertificatesByName(rows)
	}
	return out
}

func (s *Syncer) upsertCertificatesFromJobs(ctx context.Context, systemID string, jobs []wafbind.CertDomainJob, certIndexByAccount map[string]map[string]*cert.Instance) componentSyncStats {
	var certStats componentSyncStats
	enrichedCert := make(map[string]struct{})
	for _, job := range jobs {
		idx := certIndexByAccount[job.AccountName]
		if idx == nil {
			glog.Warningf("cmdb sync certificate(miss account index): system_id=%s account=%s cert_name=%q (cert list only from CLOUD_FITTER_HUAWEI_WAF_CMDB_ACCOUNT_NAMES)",
				systemID, job.AccountName, job.CertName)
			certStats.Errors++
			continue
		}
		c := idx[job.CertName]
		if c == nil {
			c = idx[strings.ToLower(job.CertName)]
		}
		if c == nil {
			glog.Warningf("cmdb sync certificate(miss name): system_id=%s account=%s cert_name=%q", systemID, job.AccountName, job.CertName)
			certStats.Errors++
			continue
		}
		ek := job.AccountName + "|" + strings.TrimSpace(c.ID)
		if _, done := enrichedCert[ek]; !done {
			accCtx := scope.WithAccountName(ctx, job.AccountName)
			if err := cert.EnrichValidityFromShow(accCtx, c); err != nil {
				glog.Warningf("cmdb sync certificate(show): system_id=%s account=%s cert_id=%s err=%v", systemID, job.AccountName, c.ID, err)
			}
			enrichedCert[ek] = struct{}{}
		}
		st := s.upsertCMDBCertificate(systemID, c, job.Domains)
		certStats.Added += st.Added
		certStats.Updated += st.Updated
		certStats.Skipped += st.Skipped
		certStats.Errors += st.Errors
	}
	return certStats
}

func accountNamesFromRows(rows []configstore.Row) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Name)
	}
	return out
}

func indexCertificatesByName(rows []*cert.Instance) map[string]*cert.Instance {
	out := make(map[string]*cert.Instance)
	for _, c := range rows {
		if c == nil {
			continue
		}
		name := strings.TrimSpace(c.Name)
		if name == "" {
			continue
		}
		if _, ok := out[name]; !ok {
			out[name] = c
		}
		lower := strings.ToLower(name)
		if _, ok := out[lower]; !ok {
			out[lower] = c
		}
	}
	return out
}

// domainNamesForCMDB 写入 CMDB「长文本 + 多值」domain_name：须为 []string，不可用顿号拼成单字符串（API is_list）。
func domainNamesForCMDB(domains []string) []string {
	seen := make(map[string]struct{}, len(domains))
	out := make([]string, 0, len(domains))
	for _, s := range domains {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// parseCMDBDomainNames 从 CMDB 读回的 domain_name（多值数组或历史顿号/逗号分隔字符串）解析为域名列表。
func parseCMDBDomainNames(row map[string]any) []string {
	if row == nil {
		return nil
	}
	v := row["domain_name"]
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return domainNamesForCMDB(t)
	case []any:
		var parts []string
		for _, x := range t {
			if s := strings.TrimSpace(anyToCompareStr(x)); s != "" {
				parts = append(parts, s)
			}
		}
		return domainNamesForCMDB(parts)
	case string:
		return splitDomainNameString(t)
	default:
		return splitDomainNameString(strings.TrimSpace(anyToCompareStr(v)))
	}
}

func splitDomainNameString(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var parts []string
	if strings.Contains(s, "、") {
		for _, p := range strings.Split(s, "、") {
			if t := strings.TrimSpace(p); t != "" {
				parts = append(parts, t)
			}
		}
	} else if strings.Contains(s, ",") {
		for _, p := range strings.Split(s, ",") {
			if t := strings.TrimSpace(p); t != "" {
				parts = append(parts, t)
			}
		}
	} else {
		parts = []string{s}
	}
	return domainNamesForCMDB(parts)
}

func domainNamesEqual(a, b []string) bool {
	a = domainNamesForCMDB(a)
	b = domainNamesForCMDB(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mergeDomainNames(existing []string, add []string) []string {
	return domainNamesForCMDB(append(append([]string{}, existing...), add...))
}

func (s *Syncer) patchCMDBCIDomainName(typePrefix, idQuery, systemID, kind, ref string, domains []string) componentSyncStats {
	st := componentSyncStats{}
	q := map[string]any{
		"q": fmt.Sprintf("%s,%s", typePrefix, idQuery),
	}
	ciID, err := s.Client.GetCIID(q)
	if err != nil {
		glog.Errorf("cmdb sync waf %s(get): system_id=%s ref=%q err=%v", kind, systemID, ref, err)
		st.Errors++
		return st
	}
	if ciID == "" {
		glog.Warningf("cmdb sync waf %s(skip missing): system_id=%s ref=%q", kind, systemID, ref)
		st.Skipped++
		return st
	}
	row, err := s.Client.GetCIFirst(q)
	if err != nil {
		glog.Errorf("cmdb sync waf %s(get row): system_id=%s ref=%q err=%v", kind, systemID, ref, err)
		st.Errors++
		return st
	}
	existing := parseCMDBDomainNames(row)
	want := domainNamesForCMDB(mergeDomainNames(existing, domains))
	if row != nil && domainNamesEqual(existing, want) {
		st.Skipped++
		return st
	}
	ciType := kind
	if row != nil {
		if t := strings.TrimSpace(fmt.Sprint(row["ci_type"])); t != "" {
			ciType = t
		} else if t := strings.TrimSpace(fmt.Sprint(row["_type"])); t != "" {
			ciType = t
		}
	}
	// domain_name 在 client 内编码为逗号分隔字符串再写入 JSON（CMDB is_list 多值长文本）
	_, err = s.Client.UpdateCI(ciID, map[string]any{
		"ci_type":     ciType,
		"domain_name": want,
	})
	if err != nil {
		glog.Errorf("cmdb sync waf %s(update domain_name): system_id=%s ref=%q id=%s err=%v", kind, systemID, ref, ciID, err)
		st.Errors++
		return st
	}
	glog.Infof("cmdb sync waf %s(update domain_name ok): system_id=%s ref=%q id=%s domains=%v", kind, systemID, ref, ciID, want)
	st.Updated++
	return st
}

// upsertCMDBCertificate 写入/更新证书 CI。唯一键为 CMDB uuid + system_id：
// uuid 填华为 SCM 证书资源 ID（如 scs1768986141935），非 RFC4122，不做 parseCloudInstanceUUID 校验。
func (s *Syncer) upsertCMDBCertificate(systemID string, c *cert.Instance, boundDomains []string) componentSyncStats {
	st := componentSyncStats{}
	if c == nil || strings.TrimSpace(c.ID) == "" {
		st.Errors++
		return st
	}
	certCloudID := strings.TrimSpace(c.ID) // SCM ListCertificates.Id
	q := map[string]any{
		"q": fmt.Sprintf("_type:certificate,uuid:%s,system_id:%s", certCloudID, systemID),
	}
	exists, err := s.Client.GetCIID(q)
	if err != nil {
		glog.Errorf("cmdb sync certificate(get): system_id=%s uuid=%s err=%v", systemID, certCloudID, err)
		st.Errors++
		return st
	}
	domainField := domainNamesForCMDB(boundDomains)
	if len(domainField) == 0 {
		if d := strings.TrimSpace(c.Domain); d != "" {
			domainField = domainNamesForCMDB([]string{d})
		}
	}
	validTo := certValidToForCMDB(c)
	if validTo == "" {
		glog.Warningf("cmdb sync certificate(empty valid_to): system_id=%s uuid=%s name=%q expire_time=%q not_after=%q",
			systemID, certCloudID, c.Name, c.ExpireTime, c.NotAfter)
	}
	fields := map[string]any{
		"certificate_name":    strings.TrimSpace(c.Name),
		"valid_from":          certValidFromForCMDB(c),
		"valid_to":            validTo,
		"signature_algorithm": strings.TrimSpace(c.SignatureAlgorithm),
		"system_id":           systemID,
		"domain_name":         domainField,
	}
	if exists != "" {
		row, err := s.Client.GetCIFirst(q)
		if err != nil {
			glog.Errorf("cmdb sync certificate(get row): system_id=%s uuid=%s err=%v", systemID, certCloudID, err)
			st.Errors++
			return st
		}
		if row != nil && !certificateCIChanged(row, fields) {
			st.Skipped++
			return st
		}
		_, err = s.Client.UpdateCI(exists, mergeAttrMaps(map[string]any{
			"ci_type":   "certificate",
			"uuid":      certCloudID,
			"system_id": systemID,
		}, fields))
		if err != nil {
			glog.Errorf("cmdb sync certificate(update): system_id=%s uuid=%s id=%s err=%v", systemID, certCloudID, exists, err)
			st.Errors++
			return st
		}
		glog.Infof("cmdb sync certificate(update ok): system_id=%s uuid=%s id=%s", systemID, certCloudID, exists)
		st.Updated++
		return st
	}
	payload := mergeAttrMaps(map[string]any{
		"uuid":      certCloudID,
		"ci_type":   "certificate",
		"system_id": systemID,
	}, fields)
	if _, err := s.Client.AddCI(payload); err != nil {
		glog.Errorf("cmdb sync certificate(add): system_id=%s uuid=%s err=%v", systemID, certCloudID, err)
		st.Errors++
		return st
	}
	glog.Infof("cmdb sync certificate(add ok): system_id=%s uuid=%s name=%q", systemID, certCloudID, c.Name)
	st.Added++
	return st
}

func certificateCIChanged(row map[string]any, want map[string]any) bool {
	for k, v := range want {
		if k == "domain_name" {
			var wantSlice []string
			switch tv := v.(type) {
			case []string:
				wantSlice = tv
			case []any:
				for _, x := range tv {
					if s := strings.TrimSpace(anyToCompareStr(x)); s != "" {
						wantSlice = append(wantSlice, s)
					}
				}
			default:
				wantSlice = splitDomainNameString(anyToCompareStr(v))
			}
			if !domainNamesEqual(parseCMDBDomainNames(row), wantSlice) {
				return true
			}
			continue
		}
		if k == "valid_from" || k == "valid_to" {
			if cmdbDateFromHuaweiTime(anyToCompareStr(row[k])) != cmdbDateFromHuaweiTime(anyToCompareStr(v)) {
				return true
			}
			continue
		}
		if strings.TrimSpace(anyToCompareStr(row[k])) != strings.TrimSpace(anyToCompareStr(v)) {
			return true
		}
	}
	return false
}

// certValidToForCMDB 失效日期：优先 ShowCertificate.not_after，其次列表 expire_time。
func certValidToForCMDB(c *cert.Instance) string {
	if s := cmdbDateFromHuaweiTime(c.NotAfter); s != "" {
		return s
	}
	return cmdbDateFromHuaweiTime(c.ExpireTime)
}

// certValidFromForCMDB 生效日期：优先 not_before，否则由失效日与有效期月数推算。
func certValidFromForCMDB(c *cert.Instance) string {
	if s := cmdbDateFromHuaweiTime(c.NotBefore); s != "" {
		return s
	}
	return certValidFromCMDB(c)
}

func cmdbDateFromHuaweiTime(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		// 已是 yyyy-MM-dd 或 yyyy-MM-dd HH:mm:ss
		if len(s) == 10 {
			return s
		}
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", s[:19], time.Local); err == nil {
			return t.Format("2006-01-02")
		}
		if t, err := time.ParseInLocation("2006-01-02", s[:10], time.Local); err == nil {
			return t.Format("2006-01-02")
		}
	}
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		sec := ms
		if sec > 1_000_000_000_000 {
			sec /= 1000
		}
		return time.Unix(sec, 0).In(time.Local).Format("2006-01-02")
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t.Format("2006-01-02")
		}
	}
	if t, err := time.ParseInLocation(layouts[0], s, time.UTC); err == nil {
		return t.In(time.Local).Format("2006-01-02")
	}
	return s
}

func certValidFromCMDB(c *cert.Instance) string {
	toStr := certValidToForCMDB(c)
	if toStr == "" || c.ValidityPeriodMonths <= 0 {
		return ""
	}
	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		return ""
	}
	return to.AddDate(0, -int(c.ValidityPeriodMonths), 0).Format("2006-01-02")
}
