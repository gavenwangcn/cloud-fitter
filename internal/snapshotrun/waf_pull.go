package snapshotrun

import (
	"context"
	"database/sql"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/configstore"
	"github.com/cloud-fitter/cloud-fitter/internal/resourcecache"
	"github.com/cloud-fitter/cloud-fitter/internal/server/cert"
	"github.com/cloud-fitter/cloud-fitter/internal/server/eip"
	"github.com/cloud-fitter/cloud-fitter/internal/server/scope"
	"github.com/cloud-fitter/cloud-fitter/internal/server/waf"
	"github.com/cloud-fitter/cloud-fitter/internal/wafbind"
)

// pullWafCertAndDomainSnapshots 从 CLOUD_FITTER_HUAWEI_WAF_CMDB_ACCOUNT_NAMES 指定账号拉 WAF/SCM 证书（不用系统关联账号），
// 写入 cloud_snap_waf* / cloud_snap_certificate 镜像表；在 EIP 快照之后调用。
func pullWafCertAndDomainSnapshots(ctx context.Context, db *sql.DB, store *configstore.Store, systemName, systemID string, eipList []*eip.Instance) error {
	cfg := wafbind.AccountNamesFromEnv()
	if len(cfg) == 0 {
		return nil
	}
	if store == nil {
		return errors.New("snapshotrun pull waf: nil store")
	}
	acco, err := store.AccountsBySystemName(systemName)
	if err != nil {
		return errors.Wrap(err, "AccountsBySystemName for waf snapshot")
	}
	linked := make([]string, 0, len(acco))
	for _, a := range acco {
		linked = append(linked, a.Name)
	}
	wafPull := wafbind.WAFAccountsForPull(cfg)
	wafbind.LogSnapshotWAFContext(systemID, systemName, cfg, linked, wafPull, len(eipList))
	if len(wafPull) == 0 {
		return nil
	}

	var allWaf []*waf.Instance
	var allCerts []*cert.Instance
	for _, accName := range wafPull {
		accCtx := scope.WithAccountName(ctx, accName)
		wafRows, werr := listTwiceIfErr(accCtx, systemID, systemName, "waf", func(c context.Context) ([]*waf.Instance, error) {
			return waf.List(c, pbtenant.CloudProvider_huawei)
		})
		if werr != nil {
			glog.Warningf("resource snapshot: waf skip account=%s system_id=%s: %v", accName, systemID, werr)
		} else {
			allWaf = append(allWaf, wafRows...)
		}
		certRows, cerr := listTwiceIfErr(accCtx, systemID, systemName, "certificate", func(c context.Context) ([]*cert.Instance, error) {
			return cert.List(c, pbtenant.CloudProvider_huawei)
		})
		if cerr != nil {
			glog.Warningf("resource snapshot: certificate skip account=%s system_id=%s: %v", accName, systemID, cerr)
		} else {
			for _, c := range certRows {
				if c == nil {
					continue
				}
				if err := cert.EnrichValidityFromShow(accCtx, c); err != nil {
					glog.Warningf("resource snapshot: cert show account=%s id=%s err=%v", accName, c.ID, err)
				}
			}
			allCerts = append(allCerts, certRows...)
		}
	}

	allWaf = wafbind.FilterWAFRows(allWaf, wafPull)
	allCerts = wafbind.FilterCertRows(allCerts, wafPull)

	if err := resourcecache.ReplaceWafSnapshot(ctx, db, systemID, systemName, allWaf); err != nil {
		return errors.Wrap(err, "replace waf snapshot")
	}
	glog.Infof("resource snapshot: waf ok system_id=%s rows=%d accounts=%v", systemID, len(allWaf), wafPull)

	if err := resourcecache.ReplaceCertificateSnapshot(ctx, db, systemID, systemName, allCerts); err != nil {
		return errors.Wrap(err, "replace certificate snapshot")
	}
			glog.Infof("resource snapshot: certificate ok system_id=%s rows=%d accounts=%v (waf env only)",
				systemID, len(allCerts), wafPull)

	bind := wafbind.Build(eipList, allWaf, wafPull)
	wafbind.LogBuildResult(systemID, len(allWaf), len(eipList), bind)
	if err := resourcecache.ReplaceWafDomainSnapshots(ctx, db, systemID, systemName, bind); err != nil {
		return errors.Wrap(err, "replace waf domain snapshot")
	}
	glog.Infof("resource snapshot: waf domain ok system_id=%s eip_bindings=%d node_bindings=%d cert_jobs=%d",
		systemID, len(bind.EIPDomains), len(bind.NodeDomains), len(bind.CertJobs))
	return nil
}
