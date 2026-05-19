package cmdb

import (
	"context"
	"fmt"
	"os"
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
)

// wafCMDBAccountNamesFromEnv 返回参与 WAF→CMDB 域名/证书同步的云账号配置名（逗号分隔）；空则不同步。
func wafCMDBAccountNamesFromEnv() []string {
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

type wafEIPBinding struct {
	domains []string
}

// syncWAFDerivedCMDB 在指定 WAF 账号上拉取防护域名，按源站 IP 匹配本系统 EIP，回写 domain_name 并同步证书 CI。
func (s *Syncer) syncWAFDerivedCMDB(ctx context.Context, systemID string, eips []*eip.Instance, linkedAccounts []configstore.Row, wafAccountNames []string) (domainSt, certStats componentSyncStats) {
	allowed := intersectAccountNames(wafAccountNames, linkedAccounts)
	if len(allowed) == 0 {
		glog.Infof("cmdb sync waf(skip): system_id=%s no WAF account overlap configured=%v linked=%v",
			systemID, wafAccountNames, accountNamesFromRows(linkedAccounts))
		return domainSt, certStats
	}

	byEIPUUID := make(map[string]*wafEIPBinding)
	bySysNode := make(map[string][]string)
	certToDomains := make(map[string][]string) // account|certName -> domains

	for _, accName := range allowed {
		accCtx := scope.WithAccountName(ctx, accName)
		wafRows, err := waf.List(accCtx, pbtenant.CloudProvider_huawei)
		if err != nil {
			glog.Warningf("cmdb sync waf(list): system_id=%s account=%s err=%v", systemID, accName, err)
			domainSt.Errors++
			continue
		}

		for _, row := range wafRows {
			if row == nil || strings.TrimSpace(row.Hostname) == "" {
				continue
			}
			domain := strings.TrimSpace(row.Hostname)
			originIPs := parseWAFOriginIPs(row.OriginServers)
			if len(originIPs) == 0 {
				continue
			}
			for _, e := range eips {
				if e == nil || e.AccountName != accName {
					continue
				}
				pub := strings.TrimSpace(e.Eip)
				if pub == "" || !sliceContains(originIPs, pub) {
					continue
				}
				eipUUID, ok := parseCloudInstanceUUID(e.EipId)
				if !ok {
					continue
				}
				sysNode := effectiveSysNodeName(eipTenantProvider(e), strings.TrimSpace(e.RegionName), e.NodeTagValue)
				if sysNode == "" {
					continue
				}
				b, ok := byEIPUUID[eipUUID]
				if !ok {
					b = &wafEIPBinding{}
					byEIPUUID[eipUUID] = b
				}
				b.domains = appendUniqueStrings(b.domains, domain)
				bySysNode[sysNode] = appendUniqueStrings(bySysNode[sysNode], domain)

				certName := strings.TrimSpace(row.CertificateName)
				if certName != "" {
					ck := accName + "|" + certName
					certToDomains[ck] = appendUniqueStrings(certToDomains[ck], domain)
				}
			}
		}
	}

	certIndexByAccount := make(map[string]map[string]*cert.Instance)
	for ck, domains := range certToDomains {
		parts := strings.SplitN(ck, "|", 2)
		if len(parts) != 2 {
			continue
		}
		accName, certName := parts[0], parts[1]
		idx, ok := certIndexByAccount[accName]
		if !ok {
			accCtx := scope.WithAccountName(ctx, accName)
			certRows, err := cert.List(accCtx, pbtenant.CloudProvider_huawei)
			if err != nil {
				glog.Warningf("cmdb sync waf(cert list): system_id=%s account=%s err=%v", systemID, accName, err)
				certStats.Errors++
				continue
			}
			idx = indexCertificatesByName(certRows)
			certIndexByAccount[accName] = idx
		}
		c := idx[certName]
		if c == nil {
			c = idx[strings.ToLower(certName)]
		}
		if c == nil {
			glog.Warningf("cmdb sync waf(cert miss): system_id=%s account=%s cert_name=%q", systemID, accName, certName)
			certStats.Errors++
			continue
		}
		st := s.upsertCMDBCertificate(systemID, c, domains)
		certStats.Added += st.Added
		certStats.Updated += st.Updated
		certStats.Skipped += st.Skipped
		certStats.Errors += st.Errors
	}

	for eipUUID, b := range byEIPUUID {
		if len(b.domains) == 0 {
			continue
		}
		st := s.patchCMDBCIDomainName("_type:EIP", fmt.Sprintf("uuid:%s,system_id:%s", eipUUID, systemID), systemID, "eip", eipUUID, b.domains)
		domainSt.Added += st.Added
		domainSt.Updated += st.Updated
		domainSt.Skipped += st.Skipped
		domainSt.Errors += st.Errors
	}
	for sysNode, domains := range bySysNode {
		st := s.patchCMDBCIDomainName("_type:system_node", fmt.Sprintf("sys_node_name:%s,system_id:%s", sysNode, systemID), systemID, "system_node", sysNode, domains)
		domainSt.Added += st.Added
		domainSt.Updated += st.Updated
		domainSt.Skipped += st.Skipped
		domainSt.Errors += st.Errors
	}

	glog.Infof("cmdb sync waf(done): system_id=%s eip_domain_bindings=%d node_bindings=%d domain(add=%d,upd=%d,skip=%d,err=%d) cert(add=%d,upd=%d,skip=%d,err=%d)",
		systemID, len(byEIPUUID), len(bySysNode),
		domainSt.Added, domainSt.Updated, domainSt.Skipped, domainSt.Errors,
		certStats.Added, certStats.Updated, certStats.Skipped, certStats.Errors)
	return domainSt, certStats
}

func intersectAccountNames(configured []string, linked []configstore.Row) []string {
	linkedSet := make(map[string]struct{}, len(linked))
	for _, r := range linked {
		if n := strings.TrimSpace(r.Name); n != "" {
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

// parseWAFOriginIPs 从 WAF 源站字段（逗号分隔的 ip:port）解析源站 IP 列表。
func parseWAFOriginIPs(originServers string) []string {
	var ips []string
	seen := make(map[string]struct{})
	for _, part := range strings.Split(originServers, ",") {
		ip := parseWAFOriginIP(strings.TrimSpace(part))
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

func parseWAFOriginIP(hostPort string) string {
	hostPort = strings.TrimSpace(hostPort)
	if hostPort == "" {
		return ""
	}
	if i := strings.LastIndex(hostPort, ":"); i > 0 {
		portPart := hostPort[i+1:]
		if _, err := strconv.Atoi(portPart); err == nil {
			return strings.TrimSpace(hostPort[:i])
		}
	}
	return hostPort
}

func sliceContains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func appendUniqueStrings(base []string, add ...string) []string {
	seen := make(map[string]struct{}, len(base)+len(add))
	for _, s := range base {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		seen[s] = struct{}{}
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

func joinDomainNamesForCMDB(domains []string) string {
	return joinSecurityGroupsForCMDB(domains)
}

func cmdbDomainNameStr(row map[string]any) string {
	v := row["domain_name"]
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case []any:
		var parts []string
		for _, x := range t {
			s := strings.TrimSpace(anyToCompareStr(x))
			if s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "、")
	default:
		return strings.TrimSpace(anyToCompareStr(v))
	}
}

func mergeDomainNames(existing string, add []string) []string {
	var all []string
	if existing != "" {
		for _, p := range strings.Split(existing, "、") {
			p = strings.TrimSpace(p)
			if p != "" {
				all = append(all, p)
			}
		}
	}
	return appendUniqueStrings(all, add...)
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
	merged := mergeDomainNames(cmdbDomainNameStr(row), domains)
	want := joinDomainNamesForCMDB(merged)
	if row != nil && cmdbDomainNameStr(row) == want {
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
	_, err = s.Client.UpdateCI(ciID, map[string]any{
		"ci_type":     ciType,
		"domain_name": want,
	})
	if err != nil {
		glog.Errorf("cmdb sync waf %s(update domain_name): system_id=%s ref=%q id=%s err=%v", kind, systemID, ref, ciID, err)
		st.Errors++
		return st
	}
	glog.Infof("cmdb sync waf %s(update domain_name ok): system_id=%s ref=%q id=%s domains=%q", kind, systemID, ref, ciID, want)
	st.Updated++
	return st
}

func (s *Syncer) upsertCMDBCertificate(systemID string, c *cert.Instance, boundDomains []string) componentSyncStats {
	st := componentSyncStats{}
	if c == nil || strings.TrimSpace(c.ID) == "" {
		st.Errors++
		return st
	}
	certUUID := strings.TrimSpace(c.ID)
	q := map[string]any{
		"q": fmt.Sprintf("_type:certificate,uuid:%s,system_id:%s", certUUID, systemID),
	}
	exists, err := s.Client.GetCIID(q)
	if err != nil {
		glog.Errorf("cmdb sync certificate(get): system_id=%s uuid=%s err=%v", systemID, certUUID, err)
		st.Errors++
		return st
	}
	domainField := joinDomainNamesForCMDB(boundDomains)
	if domainField == "" {
		domainField = strings.TrimSpace(c.Domain)
	}
	fields := map[string]any{
		"certificate_name":    strings.TrimSpace(c.Name),
		"valid_from":          certValidFromCMDB(c),
		"valid_to":            cmdbDateFromHuaweiTime(c.ExpireTime),
		"signature_algorithm": strings.TrimSpace(c.SignatureAlgorithm),
		"system_id":           systemID,
		"domain_name":         domainField,
	}
	if exists != "" {
		row, err := s.Client.GetCIFirst(q)
		if err != nil {
			glog.Errorf("cmdb sync certificate(get row): system_id=%s uuid=%s err=%v", systemID, certUUID, err)
			st.Errors++
			return st
		}
		if row != nil && !certificateCIChanged(row, fields) {
			st.Skipped++
			return st
		}
		_, err = s.Client.UpdateCI(exists, mergeAttrMaps(map[string]any{"ci_type": "certificate"}, fields))
		if err != nil {
			glog.Errorf("cmdb sync certificate(update): system_id=%s uuid=%s id=%s err=%v", systemID, certUUID, exists, err)
			st.Errors++
			return st
		}
		glog.Infof("cmdb sync certificate(update ok): system_id=%s uuid=%s id=%s", systemID, certUUID, exists)
		st.Updated++
		return st
	}
	payload := mergeAttrMaps(map[string]any{
		"uuid":      certUUID,
		"ci_type":   "certificate",
		"system_id": systemID,
	}, fields)
	if _, err := s.Client.AddCI(payload); err != nil {
		glog.Errorf("cmdb sync certificate(add): system_id=%s uuid=%s err=%v", systemID, certUUID, err)
		st.Errors++
		return st
	}
	glog.Infof("cmdb sync certificate(add ok): system_id=%s uuid=%s name=%q", systemID, certUUID, c.Name)
	st.Added++
	return st
}

func certificateCIChanged(row map[string]any, want map[string]any) bool {
	for k, v := range want {
		if cmdbDomainNameStr(row) != "" && k == "domain_name" {
			if cmdbDomainNameStr(row) != strings.TrimSpace(anyToCompareStr(v)) {
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

func cmdbDateFromHuaweiTime(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		sec := ms
		if sec > 1_000_000_000_000 {
			sec /= 1000
		}
		return time.Unix(sec, 0).Format("2006-01-02")
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return s
}

func certValidFromCMDB(c *cert.Instance) string {
	toStr := cmdbDateFromHuaweiTime(c.ExpireTime)
	if toStr == "" || c.ValidityPeriodMonths <= 0 {
		return ""
	}
	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		return ""
	}
	return to.AddDate(0, -int(c.ValidityPeriodMonths), 0).Format("2006-01-02")
}
