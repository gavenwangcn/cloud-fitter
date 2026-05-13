package snapshotrun

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbbilling"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/cmdb"
	"github.com/cloud-fitter/cloud-fitter/internal/configstore"
	"github.com/cloud-fitter/cloud-fitter/internal/resourcecache"
	"github.com/cloud-fitter/cloud-fitter/internal/server/billing"
	"github.com/cloud-fitter/cloud-fitter/internal/server/jsonapi"
)

// PullOneSystem 从云 API 全量拉取并覆盖写入快照表。某一类接口报错时跳过该类（不删不改该表已有数据）。账单按系统关联账号逐账号拉取，写入 cloud_snap_billing。
func PullOneSystem(ctx context.Context, db *sql.DB, store *configstore.Store, systemName, systemID string) error {
	if db == nil {
		return errors.New("snapshotrun PullOneSystem: nil db")
	}
	if ecsResp, err := jsonapi.ListEcsBySystemName(ctx, systemName); err != nil {
		glog.Warningf("resource snapshot: ecs skip system_id=%s name=%q: %v", systemID, systemName, err)
	} else {
		var rows []resourcecache.SnapshotRow
		for _, e := range ecsResp.GetEcses() {
			if e == nil {
				continue
			}
			key := resourcecache.ResourceKeyECS(e.GetInstanceId())
			if key == "" {
				continue
			}
			b, err := protojson.Marshal(e)
			if err != nil {
				glog.Warningf("resource snapshot: ecs marshal instance=%s: %v", e.GetInstanceId(), err)
				continue
			}
			rows = append(rows, resourcecache.SnapshotRow{
				ResourceKey: key,
				SysNodeKey:  resourcecache.SysNodeKey(e.GetProvider(), e.GetRegionName(), e.GetNodeTagValue()),
				PayloadJSON: string(b),
			})
		}
		if err := resourcecache.ReplaceSnapshotTable(ctx, db, resourcecache.TableECS, systemID, systemName, rows); err != nil {
			return errors.Wrap(err, "replace ecs snapshot")
		}
		glog.Infof("resource snapshot: ecs ok system_id=%s rows=%d", systemID, len(rows))
	}

	if rdsResp, err := jsonapi.ListRdsBySystemName(ctx, systemName); err != nil {
		glog.Warningf("resource snapshot: rds skip system_id=%s name=%q: %v", systemID, systemName, err)
	} else {
		var rows []resourcecache.SnapshotRow
		for _, r := range rdsResp.GetRdses() {
			if r == nil {
				continue
			}
			k, ok := resourcecache.MiddlewareResourceKey(r.GetInstanceId())
			if !ok {
				continue
			}
			b, err := protojson.Marshal(r)
			if err != nil {
				continue
			}
			rows = append(rows, resourcecache.SnapshotRow{
				ResourceKey: k,
				SysNodeKey:  resourcecache.SysNodeKey(r.GetProvider(), r.GetRegionName(), r.GetNodeTagValue()),
				PayloadJSON: string(b),
			})
		}
		if err := resourcecache.ReplaceSnapshotTable(ctx, db, resourcecache.TableRDS, systemID, systemName, rows); err != nil {
			return errors.Wrap(err, "replace rds snapshot")
		}
		glog.Infof("resource snapshot: rds ok system_id=%s rows=%d", systemID, len(rows))
	}

	if dcsResp, err := jsonapi.ListRedisBySystemName(ctx, systemName); err != nil {
		glog.Warningf("resource snapshot: dcs skip system_id=%s name=%q: %v", systemID, systemName, err)
	} else {
		var rows []resourcecache.SnapshotRow
		for _, r := range dcsResp.GetRedises() {
			if r == nil {
				continue
			}
			k, ok := resourcecache.MiddlewareResourceKey(r.GetInstanceId())
			if !ok {
				continue
			}
			b, err := protojson.Marshal(r)
			if err != nil {
				continue
			}
			rows = append(rows, resourcecache.SnapshotRow{
				ResourceKey: k,
				SysNodeKey:  resourcecache.SysNodeKey(r.GetProvider(), r.GetRegionName(), r.GetNodeTagValue()),
				PayloadJSON: string(b),
			})
		}
		if err := resourcecache.ReplaceSnapshotTable(ctx, db, resourcecache.TableDCS, systemID, systemName, rows); err != nil {
			return errors.Wrap(err, "replace dcs snapshot")
		}
		glog.Infof("resource snapshot: dcs ok system_id=%s rows=%d", systemID, len(rows))
	}

	if kafkaResp, err := jsonapi.ListKafkaBySystemName(ctx, systemName); err != nil {
		glog.Warningf("resource snapshot: dms/kafka skip system_id=%s name=%q: %v", systemID, systemName, err)
	} else {
		var rows []resourcecache.SnapshotRow
		for _, k := range kafkaResp.GetKafkas() {
			if k == nil {
				continue
			}
			key, ok := resourcecache.MiddlewareResourceKey(k.GetInstanceId())
			if !ok {
				continue
			}
			b, err := protojson.Marshal(k)
			if err != nil {
				continue
			}
			rows = append(rows, resourcecache.SnapshotRow{
				ResourceKey: key,
				SysNodeKey:  resourcecache.SysNodeKey(k.GetProvider(), k.GetRegionName(), k.GetNodeTagValue()),
				PayloadJSON: string(b),
			})
		}
		if err := resourcecache.ReplaceSnapshotTable(ctx, db, resourcecache.TableDMS, systemID, systemName, rows); err != nil {
			return errors.Wrap(err, "replace dms snapshot")
		}
		glog.Infof("resource snapshot: dms/kafka ok system_id=%s rows=%d", systemID, len(rows))
	}

	if cceResp, err := jsonapi.ListCceBySystemName(ctx, systemName); err != nil {
		glog.Warningf("resource snapshot: cce skip system_id=%s name=%q: %v", systemID, systemName, err)
	} else {
		var rows []resourcecache.SnapshotRow
		for _, c := range cceResp.GetClusters() {
			if c == nil {
				continue
			}
			key := resourcecache.ResourceKeyECS(c.GetClusterUid())
			if key == "" {
				continue
			}
			b, err := protojson.Marshal(c)
			if err != nil {
				continue
			}
			rows = append(rows, resourcecache.SnapshotRow{
				ResourceKey: key,
				SysNodeKey:  resourcecache.SysNodeKey(c.GetProvider(), c.GetRegionName(), c.GetNodeTagValue()),
				PayloadJSON: string(b),
			})
		}
		if err := resourcecache.ReplaceSnapshotTable(ctx, db, resourcecache.TableCCE, systemID, systemName, rows); err != nil {
			return errors.Wrap(err, "replace cce snapshot")
		}
		glog.Infof("resource snapshot: cce ok system_id=%s rows=%d", systemID, len(rows))
	}

	if eipList, err := jsonapi.ListEipBySystemName(ctx, systemName); err != nil {
		glog.Warningf("resource snapshot: eip skip system_id=%s name=%q: %v", systemID, systemName, err)
	} else {
		var rows []resourcecache.SnapshotRow
		for _, e := range eipList {
			if e == nil {
				continue
			}
			k, ok := resourcecache.ParseCloudUUID(e.EipId)
			if !ok {
				k = resourcecache.ResourceKeyECS(e.EipId)
			}
			if k == "" {
				continue
			}
			b, err := json.Marshal(e)
			if err != nil {
				continue
			}
			rows = append(rows, resourcecache.SnapshotRow{
				ResourceKey: k,
				SysNodeKey:  resourcecache.SysNodeKey(pbtenant.CloudProvider_huawei, e.RegionName, ""),
				PayloadJSON: string(b),
			})
		}
		if err := resourcecache.ReplaceSnapshotTable(ctx, db, resourcecache.TableEIP, systemID, systemName, rows); err != nil {
			return errors.Wrap(err, "replace eip snapshot")
		}
		glog.Infof("resource snapshot: eip ok system_id=%s rows=%d", systemID, len(rows))
	}

	if elbList, err := jsonapi.ListElbBySystemName(ctx, systemName); err != nil {
		glog.Warningf("resource snapshot: elb skip system_id=%s name=%q: %v", systemID, systemName, err)
	} else {
		var rows []resourcecache.SnapshotRow
		for _, e := range elbList {
			if e == nil {
				continue
			}
			k, ok := resourcecache.ParseCloudUUID(e.ID)
			if !ok {
				k = resourcecache.ResourceKeyECS(e.ID)
			}
			if k == "" {
				continue
			}
			b, err := json.Marshal(e)
			if err != nil {
				continue
			}
			rows = append(rows, resourcecache.SnapshotRow{
				ResourceKey: k,
				SysNodeKey:  resourcecache.SysNodeKey(pbtenant.CloudProvider_huawei, e.RegionName, ""),
				PayloadJSON: string(b),
			})
		}
		if err := resourcecache.ReplaceSnapshotTable(ctx, db, resourcecache.TableELB, systemID, systemName, rows); err != nil {
			return errors.Wrap(err, "replace elb snapshot")
		}
		glog.Infof("resource snapshot: elb ok system_id=%s rows=%d", systemID, len(rows))
	}

	if store == nil {
		return errors.New("snapshotrun PullOneSystem: nil store for ecs_cce map / billing")
	}
	if m, err := cmdb.HuaweiEcsIDToClusterUIDMap(ctx, store, systemName); err != nil {
		glog.Warningf("resource snapshot: ecs_cce_map skip system_id=%s name=%q: %v", systemID, systemName, err)
	} else {
		if err := resourcecache.ReplaceHuaweiEcsCceMapSnapshot(ctx, db, systemID, m); err != nil {
			return errors.Wrap(err, "replace ecs_cce map snapshot")
		}
		glog.Infof("resource snapshot: ecs_cce_map ok system_id=%s entries=%d", systemID, len(m))
	}

	billLoc, lerr := time.LoadLocation("Asia/Shanghai")
	if lerr != nil {
		billLoc = time.Local
	}
	billingMonth := time.Now().In(billLoc).Format("2006-01")
	acco, err := store.AccountsBySystemName(systemName)
	if err != nil {
		return errors.Wrap(err, "AccountsBySystemName for billing snapshot")
	}
	var billRows []resourcecache.BillingAccountRow
	for _, acc := range acco {
		resp, err := billing.ListSummary(ctx, &pbbilling.ListBillingSummaryReq{
			Provider:     pbtenant.CloudProvider(acc.Provider),
			BillingCycle: billingMonth,
			AccountName:  acc.Name,
		})
		if err != nil {
			glog.Warningf("resource snapshot: billing skip account=%s system_id=%s: %v", acc.Name, systemID, err)
			continue
		}
		b, err := protojson.Marshal(resp)
		if err != nil {
			glog.Warningf("resource snapshot: billing marshal account=%s: %v", acc.Name, err)
			continue
		}
		billRows = append(billRows, resourcecache.BillingAccountRow{
			Provider:    acc.Provider,
			AccountName: acc.Name,
			PayloadJSON: string(b),
		})
	}
	if len(billRows) > 0 {
		if err := resourcecache.ReplaceBillingSnapshot(ctx, db, systemID, systemName, billingMonth, billRows); err != nil {
			return errors.Wrap(err, "replace billing snapshot")
		}
		glog.Infof("resource snapshot: billing ok system_id=%s month=%s rows=%d", systemID, billingMonth, len(billRows))
	}

	return nil
}
