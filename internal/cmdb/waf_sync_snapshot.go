package cmdb

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/golang/glog"

	"github.com/cloud-fitter/cloud-fitter/internal/configstore"
	"github.com/cloud-fitter/cloud-fitter/internal/resourcecache"
	"github.com/cloud-fitter/cloud-fitter/internal/server/eip"
	"github.com/cloud-fitter/cloud-fitter/internal/wafbind"
)

// syncWAFDerivedCMDBFromSnapshot 从 MySQL 镜像表读取 WAF/证书（仅 CLOUD_FITTER_HUAWEI_WAF_CMDB_ACCOUNT_NAMES 账号），
// 与直连 API 相同：按源站 IP 匹配本系统 EIP 公网 IP，再写 domain_name / 证书 CI。
func (s *Syncer) syncWAFDerivedCMDBFromSnapshot(ctx context.Context, systemID string, db *sql.DB, eipList []*eip.Instance, linkedAccounts []configstore.Row, wafAccountNames []string) (domainSt, certStats componentSyncStats) {
	linked := accountNamesFromRows(linkedAccounts)
	wafPull := wafbind.WAFAccountsForPull(wafAccountNames)
	wafbind.LogCMDBWAFContext(systemID, wafAccountNames, linked, wafPull, len(eipList), true)
	if len(wafPull) == 0 {
		return domainSt, certStats
	}
	wafRows, err := resourcecache.LoadWAF(ctx, db, systemID, wafPull)
	if err != nil {
		glog.Warningf("cmdb sync waf snapshot(load waf): system_id=%s err=%v", systemID, err)
		domainSt.Errors++
		return domainSt, certStats
	}
	glog.Infof("cmdb sync waf snapshot(load): system_id=%s waf_rows=%d accounts=%v (filtered)", systemID, len(wafRows), wafPull)
	bind := wafbind.Build(eipList, wafRows, wafPull)
	wafbind.LogBuildResult(systemID, len(wafRows), len(eipList), bind)

	for _, r := range bind.EIPDomains {
		if len(r.Domains) == 0 {
			continue
		}
		st := s.patchCMDBCIDomainName("_type:EIP", fmt.Sprintf("uuid:%s,system_id:%s", r.EIPResourceKey, systemID), systemID, "eip", r.EIPResourceKey, r.Domains)
		domainSt.Added += st.Added
		domainSt.Updated += st.Updated
		domainSt.Skipped += st.Skipped
		domainSt.Errors += st.Errors
	}
	for _, r := range bind.NodeDomains {
		if len(r.Domains) == 0 {
			continue
		}
		st := s.patchCMDBCIDomainName("_type:system_node", fmt.Sprintf("sys_node_name:%s,system_id:%s", r.SysNodeKey, systemID), systemID, "system_node", r.SysNodeKey, r.Domains)
		domainSt.Added += st.Added
		domainSt.Updated += st.Updated
		domainSt.Skipped += st.Skipped
		domainSt.Errors += st.Errors
	}

	certRows, err := resourcecache.LoadCertificates(ctx, db, systemID, wafPull)
	if err != nil {
		glog.Warningf("cmdb sync waf snapshot(load cert): system_id=%s err=%v", systemID, err)
		certStats.Errors++
		return domainSt, certStats
	}
	certByAccount := certIndexFromSnapshot(certRows, wafPull)
	glog.Infof("cmdb sync certificate(snapshot): system_id=%s cert_rows=%d waf_accounts=%v indexed_accounts=%d",
		systemID, len(certRows), wafPull, len(certByAccount))
	certStats = s.upsertCertificatesFromJobs(ctx, systemID, bind.CertJobs, certByAccount)

	glog.Infof("cmdb sync waf snapshot(done): system_id=%s eip_bindings=%d node_bindings=%d cert_jobs=%d",
		systemID, len(bind.EIPDomains), len(bind.NodeDomains), len(bind.CertJobs))
	return domainSt, certStats
}
