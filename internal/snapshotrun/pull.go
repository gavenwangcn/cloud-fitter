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
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbcce"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbecs"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbkafka"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbrds"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbredis"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/cmdb"
	"github.com/cloud-fitter/cloud-fitter/internal/configstore"
	"github.com/cloud-fitter/cloud-fitter/internal/resourcecache"
	"github.com/cloud-fitter/cloud-fitter/internal/server/billing"
	"github.com/cloud-fitter/cloud-fitter/internal/server/eip"
	"github.com/cloud-fitter/cloud-fitter/internal/server/elb"
	"github.com/cloud-fitter/cloud-fitter/internal/server/jsonapi"
)

// PullOneSystem 从云 API 全量拉取并覆盖写入快照表。某一类接口报错时跳过该类（不删不改该表已有数据）。
// 每一类拉取若失败会再重试 1 次（间隔见 snapshotCategoryRetryBackoff），仍失败则跳过该类。
// 某一类接口成功但返回空列表时也不删不改（保留上次非空快照）。账单仅在有成功拉取的账号行时才写库。
func PullOneSystem(ctx context.Context, db *sql.DB, store *configstore.Store, systemName, systemID string) error {
	if db == nil {
		return errors.New("snapshotrun PullOneSystem: nil db")
	}
	if ecsResp, err := listTwiceIfErr(ctx, systemID, systemName, "ecs", func(c context.Context) (*pbecs.ListResp, error) {
		return jsonapi.ListEcsBySystemName(c, systemName)
	}); err != nil {
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
		if len(rows) > 0 {
			glog.Infof("resource snapshot: ecs ok system_id=%s rows=%d", systemID, len(rows))
		} else {
			glog.Infof("resource snapshot: ecs empty list, keep existing snapshot system_id=%s name=%q", systemID, systemName)
		}
	}

	if rdsResp, err := listTwiceIfErr(ctx, systemID, systemName, "rds", func(c context.Context) (*pbrds.ListResp, error) {
		return jsonapi.ListRdsBySystemName(c, systemName)
	}); err != nil {
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
		if len(rows) > 0 {
			glog.Infof("resource snapshot: rds ok system_id=%s rows=%d", systemID, len(rows))
		} else {
			glog.Infof("resource snapshot: rds empty list, keep existing snapshot system_id=%s name=%q", systemID, systemName)
		}
	}

	if dcsResp, err := listTwiceIfErr(ctx, systemID, systemName, "dcs", func(c context.Context) (*pbredis.ListResp, error) {
		return jsonapi.ListRedisBySystemName(c, systemName)
	}); err != nil {
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
		if len(rows) > 0 {
			glog.Infof("resource snapshot: dcs ok system_id=%s rows=%d", systemID, len(rows))
		} else {
			glog.Infof("resource snapshot: dcs empty list, keep existing snapshot system_id=%s name=%q", systemID, systemName)
		}
	}

	if kafkaResp, err := listTwiceIfErr(ctx, systemID, systemName, "dms/kafka", func(c context.Context) (*pbkafka.ListResp, error) {
		return jsonapi.ListKafkaBySystemName(c, systemName)
	}); err != nil {
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
		if len(rows) > 0 {
			glog.Infof("resource snapshot: dms/kafka ok system_id=%s rows=%d", systemID, len(rows))
		} else {
			glog.Infof("resource snapshot: dms/kafka empty list, keep existing snapshot system_id=%s name=%q", systemID, systemName)
		}
	}

	if cceResp, err := listTwiceIfErr(ctx, systemID, systemName, "cce", func(c context.Context) (*pbcce.ListResp, error) {
		return jsonapi.ListCceBySystemName(c, systemName)
	}); err != nil {
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
		if len(rows) > 0 {
			glog.Infof("resource snapshot: cce ok system_id=%s rows=%d", systemID, len(rows))
		} else {
			glog.Infof("resource snapshot: cce empty list, keep existing snapshot system_id=%s name=%q", systemID, systemName)
		}
	}

	if eipList, err := listTwiceIfErr(ctx, systemID, systemName, "eip", func(c context.Context) ([]*eip.Instance, error) {
		return jsonapi.ListEipBySystemName(c, systemName)
	}); err != nil {
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
				SysNodeKey:  resourcecache.SysNodeKey(pbtenant.CloudProvider_huawei, e.RegionName, e.NodeTagValue),
				PayloadJSON: string(b),
			})
		}
		if err := resourcecache.ReplaceSnapshotTable(ctx, db, resourcecache.TableEIP, systemID, systemName, rows); err != nil {
			return errors.Wrap(err, "replace eip snapshot")
		}
		if len(rows) > 0 {
			glog.Infof("resource snapshot: eip ok system_id=%s rows=%d", systemID, len(rows))
		} else {
			glog.Infof("resource snapshot: eip empty list, keep existing snapshot system_id=%s name=%q", systemID, systemName)
		}
	}

	if elbList, err := listTwiceIfErr(ctx, systemID, systemName, "elb", func(c context.Context) ([]*elb.Instance, error) {
		return jsonapi.ListElbBySystemName(c, systemName)
	}); err != nil {
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
				SysNodeKey:  resourcecache.SysNodeKey(pbtenant.CloudProvider_huawei, e.RegionName, e.NodeTagValue),
				PayloadJSON: string(b),
			})
		}
		if err := resourcecache.ReplaceSnapshotTable(ctx, db, resourcecache.TableELB, systemID, systemName, rows); err != nil {
			return errors.Wrap(err, "replace elb snapshot")
		}
		if len(rows) > 0 {
			glog.Infof("resource snapshot: elb ok system_id=%s rows=%d", systemID, len(rows))
		} else {
			glog.Infof("resource snapshot: elb empty list, keep existing snapshot system_id=%s name=%q", systemID, systemName)
		}
	}

	if store == nil {
		return errors.New("snapshotrun PullOneSystem: nil store for ecs_cce map / billing")
	}
	if m, err := listTwiceIfErr(ctx, systemID, systemName, "ecs_cce_map", func(c context.Context) (map[string]string, error) {
		return cmdb.HuaweiEcsIDToClusterUIDMap(c, store, systemName)
	}); err != nil {
		glog.Warningf("resource snapshot: ecs_cce_map skip system_id=%s name=%q: %v", systemID, systemName, err)
	} else {
		if err := resourcecache.ReplaceHuaweiEcsCceMapSnapshot(ctx, db, systemID, m); err != nil {
			return errors.Wrap(err, "replace ecs_cce map snapshot")
		}
		if len(m) > 0 {
			glog.Infof("resource snapshot: ecs_cce_map ok system_id=%s entries=%d", systemID, len(m))
		} else {
			glog.Infof("resource snapshot: ecs_cce_map empty map, keep existing snapshot system_id=%s name=%q", systemID, systemName)
		}
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
		acc := acc
		resp, err := listTwiceIfErr(ctx, systemID, systemName, "billing:"+acc.Name, func(c context.Context) (*pbbilling.ListBillingSummaryResp, error) {
			return billing.ListSummary(c, &pbbilling.ListBillingSummaryReq{
				Provider:     pbtenant.CloudProvider(acc.Provider),
				BillingCycle: billingMonth,
				AccountName:  acc.Name,
			})
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
