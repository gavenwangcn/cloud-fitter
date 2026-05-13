package cmdb

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbbilling"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbcce"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbecs"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbkafka"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbrds"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbredis"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbutilization"
	"github.com/cloud-fitter/cloud-fitter/internal/billingagg"
	"github.com/cloud-fitter/cloud-fitter/internal/configstore"
	"github.com/cloud-fitter/cloud-fitter/internal/resourcecache"
	"github.com/cloud-fitter/cloud-fitter/internal/server/billing"
	"github.com/cloud-fitter/cloud-fitter/internal/server/eip"
	"github.com/cloud-fitter/cloud-fitter/internal/server/elb"
	"github.com/cloud-fitter/cloud-fitter/internal/server/jsonapi"
)

// Syncer 将 cloud-fitter 拉取逻辑（与 jsonapi 按 systemName 一致）与 CMDB 开放 API 写入步骤对齐。
type Syncer struct {
	Client *Client
	Store  *configstore.Store
}

type componentSyncStats struct {
	Added   int
	Updated int
	Skipped int
	Deleted int
	Errors  int
}

type systemSyncStats struct {
	SystemNode componentSyncStats
	K8s        componentSyncStats
	Host       componentSyncStats
	Middleware componentSyncStats
	EIP        componentSyncStats
	ELB        componentSyncStats
	Billing    componentSyncStats
}

// Run 与 Python main.cmdb 一致：分页拉取 CMDB 中全部 system，再对每个 system_id 同步。
// 云资源默认来自本服务 List*BySystemName；若设置 CLOUD_FITTER_CMDB_USE_RESOURCE_SNAPSHOT=true 且使用 MySQL，则从 cloud_snap_* 快照表读取（与定时快照任务配合）。
func (s *Syncer) Run(ctx context.Context) error {
	if s == nil || s.Client == nil || s.Store == nil {
		return errors.New("cmdb syncer: nil client or store")
	}
	glog.Infof("cmdb sync run(start): full scheduled sync")
	ids, err := s.listCMDBSystemIDs()
	if err != nil {
		return err
	}
	glog.Infof("cmdb sync: found %d system(s) in CMDB", len(ids))
	for _, systemID := range ids {
		if err := s.syncSystem(ctx, systemID); err != nil {
			glog.Warningf("cmdb sync system_id=%s: %v", systemID, err)
		}
	}
	glog.Infof("cmdb sync run(done): full scheduled sync finished")
	return nil
}

func (s *Syncer) listCMDBSystemIDs() ([]string, error) {
	var out []string
	page := 1
	for {
		data, err := s.Client.GetCI(map[string]any{
			"q":    "_type:system",
			"page": page,
		})
		if err != nil {
			return nil, err
		}
		res, _ := data["result"].([]any)
		if len(res) == 0 {
			break
		}
		for _, it := range res {
			row, _ := it.(map[string]any)
			if row == nil {
				continue
			}
			sid := row["system_id"]
			if sid == nil {
				continue
			}
			out = append(out, fmt.Sprint(sid))
		}
		page++
	}
	return out, nil
}

func (s *Syncer) syncSystem(ctx context.Context, systemID string) error {
	sysRow, err := s.Store.SystemBySystemID(systemID)
	if err != nil {
		glog.Infof("cmdb sync: skip system_id=%s (no local system row: %v)", systemID, err)
		return nil
	}
	if len(sysRow.AccountIDs) == 0 {
		glog.Infof("cmdb sync: skip system_id=%s name=%q (no linked cloud accounts)", systemID, sysRow.Name)
		return nil
	}
	systemName := sysRow.Name
	cmdbSystemName := systemName
	if n, err := s.Client.GetSystemNameBySystemID(systemID); err != nil {
		glog.Warningf("cmdb sync: get cmdb system_name by system_id=%s failed: %v (fallback local name)", systemID, err)
	} else if strings.TrimSpace(n) != "" {
		cmdbSystemName = strings.TrimSpace(n)
	}

	useSnap := resourcecache.CMDBUseResourceSnapshotFromEnv() && configstore.UseMySQLFromEnv()
	dbSnap := s.Store.SQLDB()

	var ecsResp *pbecs.ListResp
	var rdsResp *pbrds.ListResp
	var redisResp *pbredis.ListResp
	var kafkaResp *pbkafka.ListResp
	var cceResp *pbcce.ListResp
	var eipList []*eip.Instance
	var elbList []*elb.Instance

	if useSnap && dbSnap != nil {
		var err error
		if ecsResp, err = resourcecache.LoadECS(ctx, dbSnap, systemID); err != nil {
			return errors.Wrap(err, "LoadECS snapshot")
		}
		if rdsResp, err = resourcecache.LoadRDS(ctx, dbSnap, systemID); err != nil {
			return errors.Wrap(err, "LoadRDS snapshot")
		}
		if redisResp, err = resourcecache.LoadRedis(ctx, dbSnap, systemID); err != nil {
			return errors.Wrap(err, "LoadRedis snapshot")
		}
		if kafkaResp, err = resourcecache.LoadKafka(ctx, dbSnap, systemID); err != nil {
			return errors.Wrap(err, "LoadKafka snapshot")
		}
		if cceResp, err = resourcecache.LoadCCE(ctx, dbSnap, systemID); err != nil {
			return errors.Wrap(err, "LoadCCE snapshot")
		}
		if eipList, err = resourcecache.LoadEIP(ctx, dbSnap, systemID); err != nil {
			return errors.Wrap(err, "LoadEIP snapshot")
		}
		if elbList, err = resourcecache.LoadELB(ctx, dbSnap, systemID); err != nil {
			return errors.Wrap(err, "LoadELB snapshot")
		}
		glog.Infof("cmdb sync system_id=%s: mysql snapshots ecs=%d rds=%d dcs=%d kafka=%d cce=%d eip=%d elb=%d (billing from snapshot when enabled)",
			systemID, len(ecsResp.GetEcses()), len(rdsResp.GetRdses()), len(redisResp.GetRedises()), len(kafkaResp.GetKafkas()), len(cceResp.GetClusters()), len(eipList), len(elbList))
	} else {
		if useSnap && dbSnap == nil {
			glog.Warningf("cmdb sync system_id=%s: CLOUD_FITTER_CMDB_USE_RESOURCE_SNAPSHOT on but store has no SQL db, using cloud API", systemID)
		}
		var err error
		ecsResp, err = jsonapi.ListEcsBySystemName(ctx, systemName)
		if err != nil {
			return errors.Wrap(err, "ListEcsBySystemName")
		}
		rdsResp, err = jsonapi.ListRdsBySystemName(ctx, systemName)
		if err != nil {
			return errors.Wrap(err, "ListRdsBySystemName")
		}
		redisResp, err = jsonapi.ListRedisBySystemName(ctx, systemName)
		if err != nil {
			return errors.Wrap(err, "ListRedisBySystemName")
		}
		kafkaResp, err = jsonapi.ListKafkaBySystemName(ctx, systemName)
		if err != nil {
			return errors.Wrap(err, "ListKafkaBySystemName")
		}
		cceResp, err = jsonapi.ListCceBySystemName(ctx, systemName)
		if err != nil {
			return errors.Wrap(err, "ListCceBySystemName")
		}
		eipList, err = jsonapi.ListEipBySystemName(ctx, systemName)
		if err != nil {
			return errors.Wrap(err, "ListEipBySystemName")
		}
		elbList, err = jsonapi.ListElbBySystemName(ctx, systemName)
		if err != nil {
			return errors.Wrap(err, "ListElbBySystemName")
		}
	}
	billLoc, lerr := time.LoadLocation("Asia/Shanghai")
	if lerr != nil {
		billLoc = time.Local
	}
	billingMonth := time.Now().In(billLoc).Format("2006-01")
	acco, err := s.Store.AccountsBySystemName(systemName)
	if err != nil {
		return errors.Wrap(err, "AccountsBySystemName")
	}

	// 华为 ECS→CCE 集群 UID：非快照模式调云 API；快照模式读 cloud_snap_ecs_cce_map（定时快照任务内写入）。
	var ecsIDToCceUID map[string]string
	if useSnap && dbSnap != nil {
		var lerr error
		ecsIDToCceUID, lerr = resourcecache.LoadHuaweiEcsCceMap(ctx, dbSnap, systemID)
		if lerr != nil {
			return errors.Wrap(lerr, "LoadHuaweiEcsCceMap snapshot")
		}
	} else {
		var err error
		ecsIDToCceUID, err = HuaweiEcsIDToClusterUIDMap(ctx, s.Store, systemName)
		if err != nil {
			glog.Warningf("cmdb sync: huawei ecs->cce uid map: %v (host_ip_new may be empty)", err)
			ecsIDToCceUID = nil
		}
	}

	systemNodes := collectSystemNodeKeys(ecsResp, rdsResp, redisResp, kafkaResp, cceResp)
	mergeEIPSystemNodeKeys(systemNodes, eipList)
	mergeELBSystemNodeKeys(systemNodes, elbList)
	hosts := buildHosts(ecsResp, ecsIDToCceUID)
	k8sList := buildK8sClusters(cceResp, hosts)
	middlewares := buildMiddlewares(rdsResp, redisResp, kafkaResp)

	clusterToECS := hostBelongsK8S(k8sList, hosts)

	stats := systemSyncStats{}
	stats.SystemNode = s.addCMDBSystemNodes(systemID, cmdbSystemName, systemNodes)
	stats.K8s = s.addCMDBK8sClusters(systemID, k8sList, clusterToECS)
	if len(k8sList) > 0 {
		r := s.reconcileCMDBCIsNotInAPI("k8s_cluster", systemID, uuidSetFromK8s(k8sList), nil)
		stats.K8s.Deleted += r.Deleted
		stats.K8s.Errors += r.Errors
	}
	stats.Host = s.addCMDBHosts(systemID, hosts)
	if len(hosts) > 0 {
		r := s.reconcileCMDBCIsNotInAPI("server", systemID, uuidSetFromHosts(hosts), nil)
		stats.Host.Deleted += r.Deleted
		stats.Host.Errors += r.Errors
	}
	stats.Middleware = s.addCMDBMiddlewares(systemID, middlewares)
	if len(middlewares) > 0 {
		r := s.reconcileCMDBCIsNotInAPI("middle_software", systemID, uuidSetFromMiddlewares(middlewares), []string{"RDS_INS", "DCS_REDIS", "DMS_ROCKETMQ"})
		stats.Middleware.Deleted += r.Deleted
		stats.Middleware.Errors += r.Errors
	}
	stats.EIP = s.addCMDBEIPs(systemID, eipList)
	if len(eipList) > 0 {
		r := s.reconcileCMDBCIsNotInAPI("EIP", systemID, uuidSetFromEIPs(eipList), nil)
		stats.EIP.Deleted += r.Deleted
		stats.EIP.Errors += r.Errors
	}
	stats.ELB = s.addCMDBELBs(systemID, elbList)
	if len(elbList) > 0 {
		r := s.reconcileCMDBCIsNotInAPI("ELB", systemID, uuidSetFromELBs(elbList), nil)
		stats.ELB.Deleted += r.Deleted
		stats.ELB.Errors += r.Errors
	}
	stats.Billing = componentSyncStats{}
	for _, acc := range acco {
		var billResp *pbbilling.ListBillingSummaryResp
		if useSnap && dbSnap != nil {
			var lerr error
			billResp, lerr = resourcecache.LoadBilling(ctx, dbSnap, systemID, billingMonth, acc.Provider, acc.Name)
			if lerr != nil {
				return errors.Wrapf(lerr, "LoadBilling snapshot account=%s", acc.Name)
			}
		} else {
			var lerr error
			billResp, lerr = billing.ListSummary(ctx, &pbbilling.ListBillingSummaryReq{
				Provider:     pbtenant.CloudProvider(acc.Provider),
				BillingCycle: billingMonth,
				AccountName:  acc.Name,
			})
			if lerr != nil {
				return errors.Wrapf(lerr, "billing ListSummary account=%s", acc.Name)
			}
		}
		st := s.addCMDBBillings(systemID, billingMonth, acc.Name, billResp)
		stats.Billing.Added += st.Added
		stats.Billing.Updated += st.Updated
		stats.Billing.Skipped += st.Skipped
		stats.Billing.Deleted += st.Deleted
		stats.Billing.Errors += st.Errors
	}
	glog.Infof(
		"cmdb sync system(done): system_id=%s system_name=%q stats system_node(add=%d,upd=%d,skip=%d,err=%d) k8s(add=%d,upd=%d,skip=%d,del=%d,err=%d) host(add=%d,upd=%d,skip=%d,del=%d,err=%d) middleware(add=%d,upd=%d,skip=%d,del=%d,err=%d) eip(add=%d,upd=%d,skip=%d,del=%d,err=%d) elb(add=%d,upd=%d,skip=%d,del=%d,err=%d) billing(add=%d,upd=%d,skip=%d,del=%d,err=%d)",
		systemID, systemName,
		stats.SystemNode.Added, stats.SystemNode.Updated, stats.SystemNode.Skipped, stats.SystemNode.Errors,
		stats.K8s.Added, stats.K8s.Updated, stats.K8s.Skipped, stats.K8s.Deleted, stats.K8s.Errors,
		stats.Host.Added, stats.Host.Updated, stats.Host.Skipped, stats.Host.Deleted, stats.Host.Errors,
		stats.Middleware.Added, stats.Middleware.Updated, stats.Middleware.Skipped, stats.Middleware.Deleted, stats.Middleware.Errors,
		stats.EIP.Added, stats.EIP.Updated, stats.EIP.Skipped, stats.EIP.Deleted, stats.EIP.Errors,
		stats.ELB.Added, stats.ELB.Updated, stats.ELB.Skipped, stats.ELB.Deleted, stats.ELB.Errors,
		stats.Billing.Added, stats.Billing.Updated, stats.Billing.Skipped, stats.Billing.Deleted, stats.Billing.Errors,
	)
	return nil
}

// SyncOneBySystemName 按本库系统名称对 CMDB 做该系统的全量同步，逻辑与 Run 中针对单系统的处理一致（syncSystem）。
// 先校验 CMDB 中存在与本地相同的 system_id，否则返回「CMDB中没有相同系统信息」。
func (s *Syncer) SyncOneBySystemName(ctx context.Context, systemName string) error {
	if s == nil || s.Client == nil || s.Store == nil {
		return errors.New("cmdb syncer: nil client or store")
	}
	name := strings.TrimSpace(systemName)
	if name == "" {
		return errors.New("系统名称不能为空")
	}
	sysRow, err := s.Store.SystemByName(name)
	if err != nil {
		return err
	}
	if len(sysRow.AccountIDs) == 0 {
		return errors.New("本系统未关联云账号，无法同步")
	}
	ok, err := s.Client.SystemIDExistsInCMDB(sysRow.SystemID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("CMDB中没有相同系统信息")
	}
	glog.Infof("cmdb sync run(start): manual single sync system_name=%q system_id=%s", sysRow.Name, sysRow.SystemID)
	return s.syncSystem(ctx, sysRow.SystemID)
}

func collectSystemNodeKeys(
	ecsResp *pbecs.ListResp,
	rdsResp *pbrds.ListResp,
	redisResp *pbredis.ListResp,
	kafkaResp *pbkafka.ListResp,
	cceResp *pbcce.ListResp,
) map[string]struct{} {
	out := make(map[string]struct{})
	add := func(p pbtenant.CloudProvider, region, nodeTag string) {
		k := effectiveSysNodeName(p, region, nodeTag)
		if k == "" {
			return
		}
		out[k] = struct{}{}
	}
	if ecsResp != nil {
		for _, e := range ecsResp.Ecses {
			add(e.GetProvider(), e.GetRegionName(), e.GetNodeTagValue())
		}
	}
	if rdsResp != nil {
		for _, r := range rdsResp.Rdses {
			add(r.GetProvider(), r.GetRegionName(), r.GetNodeTagValue())
		}
	}
	if redisResp != nil {
		for _, r := range redisResp.Redises {
			add(r.GetProvider(), r.GetRegionName(), r.GetNodeTagValue())
		}
	}
	if kafkaResp != nil {
		for _, k := range kafkaResp.Kafkas {
			add(k.GetProvider(), k.GetRegionName(), k.GetNodeTagValue())
		}
	}
	if cceResp != nil {
		for _, c := range cceResp.Clusters {
			add(c.GetProvider(), c.GetRegionName(), c.GetNodeTagValue())
		}
	}
	return out
}

// parseCloudInstanceUUID 校验云 API 返回的实例 ID 是否为 RFC4122 UUID，并规范化为标准字符串，用于 CMDB uuid 字段与 GetCI 查询。
func parseCloudInstanceUUID(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := uuid.Parse(raw)
	if err != nil {
		return "", false
	}
	return u.String(), true
}

// namespaceMiddlewareNonRFC 用于将「非标准 UUID」的云实例 ID（如华为 RDS 带后缀）映射为确定性 RFC4122 UUID（SHA1 / UUID v5）。
var namespaceMiddlewareNonRFC = uuid.MustParse("8f7e6d5c-4b3a-2918-0f1e-2d3c4b5a6978")

// middlewareCMDBUUID 生成写入 CMDB middle_software.uuid 的值：若云返回已是合法 UUID 则规范化；
// 否则用 UUID v5 稳定映射，便于与云实例 ID 一一对应。
func middlewareCMDBUUID(instanceID string) (string, bool) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return "", false
	}
	if u, err := uuid.Parse(instanceID); err == nil {
		return u.String(), true
	}
	u := uuid.NewSHA1(namespaceMiddlewareNonRFC, []byte(instanceID))
	return u.String(), true
}

func uuidSetFromHosts(hosts []hostRec) map[string]struct{} {
	m := make(map[string]struct{})
	for _, h := range hosts {
		if u, ok := parseCloudInstanceUUID(h.InstanceID); ok {
			m[u] = struct{}{}
		}
	}
	return m
}

func uuidSetFromMiddlewares(mws []mwRec) map[string]struct{} {
	m := make(map[string]struct{})
	for _, x := range mws {
		if u, ok := middlewareCMDBUUID(x.InstanceID); ok {
			m[u] = struct{}{}
		}
	}
	return m
}

func uuidSetFromK8s(k8s []k8sCluster) map[string]struct{} {
	m := make(map[string]struct{})
	for _, c := range k8s {
		if u, ok := parseCloudInstanceUUID(c.ClusterUID); ok {
			m[u] = struct{}{}
		}
	}
	return m
}

func uuidSetFromEIPs(eips []*eip.Instance) map[string]struct{} {
	m := make(map[string]struct{})
	for _, e := range eips {
		if e == nil {
			continue
		}
		if u, ok := parseCloudInstanceUUID(e.EipId); ok {
			m[u] = struct{}{}
		}
	}
	return m
}

func uuidSetFromELBs(elbs []*elb.Instance) map[string]struct{} {
	m := make(map[string]struct{})
	for _, e := range elbs {
		if e == nil {
			continue
		}
		if u, ok := parseCloudInstanceUUID(e.ID); ok {
			m[u] = struct{}{}
		}
	}
	return m
}

// reconcileCMDBCIsNotInAPI 在接口已成功返回且本次列表可构造出非空 keep（CMDB uuid / middle_software 与 middlewareCMDBUUID 一致）时，删除 CMDB 中同 _type、同 system_id 但 uuid 不在接口集合中的 CI。
// 接口列表为空或无任何合法 UUID 时不删除（避免误清空）。middle_software 仅处理 resource_type 在 mwTypes 内的行。
func (s *Syncer) reconcileCMDBCIsNotInAPI(ciType, systemID string, keep map[string]struct{}, mwTypes []string) componentSyncStats {
	st := componentSyncStats{}
	if s == nil || s.Client == nil || len(keep) == 0 {
		return st
	}
	systemID = strings.TrimSpace(systemID)
	if systemID == "" {
		return st
	}
	page := 1
	for {
		data, err := s.Client.GetCI(map[string]any{
			"q":    fmt.Sprintf("_type:%s,system_id:%s", ciType, systemID),
			"page": page,
		})
		if err != nil {
			glog.Errorf("cmdb reconcile list(%s): system_id=%s page=%d err=%v", ciType, systemID, page, err)
			st.Errors++
			return st
		}
		res, _ := data["result"].([]any)
		if len(res) == 0 {
			break
		}
		for _, it := range res {
			row, _ := it.(map[string]any)
			if row == nil {
				continue
			}
			if len(mwTypes) > 0 {
				rt := strings.TrimSpace(fmt.Sprint(row["resource_type"]))
				match := false
				for _, t := range mwTypes {
					if rt == t {
						match = true
						break
					}
				}
				if !match {
					continue
				}
			}
			canon, ok := parseCloudInstanceUUID(fmt.Sprint(row["uuid"]))
			if !ok {
				continue
			}
			if _, in := keep[canon]; in {
				continue
			}
			ciID := fmt.Sprint(row["_id"])
			if strings.TrimSpace(ciID) == "" {
				continue
			}
			if _, err := s.Client.DeleteCI(ciID); err != nil {
				glog.Errorf("cmdb reconcile delete: system_id=%s _type=%s ci_id=%s uuid=%s err=%v", systemID, ciType, ciID, canon, err)
				st.Errors++
				continue
			}
			glog.Infof("cmdb reconcile delete ok: system_id=%s _type=%s ci_id=%s uuid=%s", systemID, ciType, ciID, canon)
			st.Deleted++
		}
		page++
	}
	return st
}

// effectiveSysNodeName 有「节点」标签时用车载标签值作为 CMDB 系统节点名；否则为「云中文名-地域」。
func effectiveSysNodeName(p pbtenant.CloudProvider, region, nodeTagValue string) string {
	if s := strings.TrimSpace(nodeTagValue); s != "" {
		return s
	}
	region = strings.TrimSpace(region)
	if region == "" {
		return ""
	}
	return cloudTypeLabel(p) + "-" + region
}

// splitSysNodeNameForCMDB 解析默认格式「{云名}-{地域}」为 cloud_type + location；自定义标签时整段作为 cloud_type，location 为空。
func splitSysNodeNameForCMDB(node string) (cloudType, location string) {
	node = strings.TrimSpace(node)
	if node == "" {
		return "", ""
	}
	parts := strings.SplitN(node, "-", 2)
	if len(parts) < 2 {
		return node, ""
	}
	for _, p := range []string{"华为云", "阿里云", "腾讯云", "AWS", "云"} {
		if parts[0] == p {
			return parts[0], parts[1]
		}
	}
	return node, ""
}

func cmdbCloudLocationFromSysNodeName(sysNodeName, fallbackCloud, fallbackLoc string) (cloudType, location string) {
	ct, loc := splitSysNodeNameForCMDB(sysNodeName)
	if loc != "" {
		return ct, loc
	}
	if ct != "" {
		return ct, ""
	}
	return fallbackCloud, fallbackLoc
}

type k8sCluster struct {
	Name, Version, CloudLabel, Region, ClusterUID, SysNodeName string
	Environment, Remarks string
}

// k8sClusterEnvFromHosts 用挂载到该集群（CceCluster.cluster_uid）的 ECS 环境标签去重拼接，与快照中 ECS+CCE 镜像数据一致。
func k8sClusterEnvFromHosts(clusterUID string, hosts []hostRec) string {
	clusterUID = strings.TrimSpace(clusterUID)
	if clusterUID == "" {
		return ""
	}
	var parts []string
	seen := make(map[string]struct{})
	for _, h := range hosts {
		if strings.TrimSpace(h.CceClusterID) != clusterUID {
			continue
		}
		ev := strings.TrimSpace(h.EnvTagValue)
		if ev == "" {
			continue
		}
		if _, ok := seen[ev]; ok {
			continue
		}
		seen[ev] = struct{}{}
		parts = append(parts, ev)
	}
	return strings.Join(parts, "、")
}

// k8sRemarksFromCCE 用 CCE 列表/快照中已有字段拼备注（状态、规格、节点与容量），不依赖 proto 变更。
func k8sRemarksFromCCE(c *pbcce.CceCluster) string {
	if c == nil {
		return ""
	}
	var parts []string
	if s := strings.TrimSpace(c.GetPhase()); s != "" {
		parts = append(parts, "状态:"+s)
	}
	if s := strings.TrimSpace(c.GetFlavor()); s != "" {
		parts = append(parts, "规格:"+s)
	}
	if c.GetNodeTotal() > 0 {
		parts = append(parts, fmt.Sprintf("节点:%d/%d", c.GetNodeNormal(), c.GetNodeTotal()))
	}
	if c.GetCpuTotal() > 0 {
		parts = append(parts, fmt.Sprintf("vCPU:%d", c.GetCpuTotal()))
	}
	if c.GetMemoryTotalMb() > 0 {
		parts = append(parts, fmt.Sprintf("内存:%dMB", c.GetMemoryTotalMb()))
	}
	return strings.Join(parts, "、")
}

func buildK8sClusters(resp *pbcce.ListResp, hosts []hostRec) []k8sCluster {
	if resp == nil {
		return nil
	}
	var out []k8sCluster
	for _, c := range resp.Clusters {
		if c == nil {
			continue
		}
		uid := strings.TrimSpace(c.GetClusterUid())
		out = append(out, k8sCluster{
			Name:        c.GetClusterName(),
			Version:     c.GetK8SVersion(),
			CloudLabel:  cloudTypeLabel(c.GetProvider()),
			Region:      c.GetRegionName(),
			ClusterUID:  uid,
			SysNodeName: effectiveSysNodeName(c.GetProvider(), c.GetRegionName(), c.GetNodeTagValue()),
			Environment: k8sClusterEnvFromHosts(uid, hosts),
			Remarks:     k8sRemarksFromCCE(c),
		})
	}
	return out
}

type hostRec struct {
	InstanceID                                string // 云 ECS 实例 ID（华为等为 UUID）
	Name, IP, CloudLabel, Region, SysNodeName string
	CPU                                       int32
	MemGBStr                                  string
	DiskStr                                   string // 磁盘：系统盘/数据盘GB 有任一则为「a+b」；仅两者皆无(0)时用 disk_summary，再无则空
	StorageAttr                               string // CMDB storage：系统盘/数据盘合计（与控制台列一致）
	OS                                        string
	CceClusterID                              string
	EnvTagValue                               string // 环境标签（与 ECS 一致）；用于推导同集群 K8s CI 的 environment
	CpuPeak30, CpuPeak180                     string
	CpuAvg30, CpuAvg180                       string
	MemPeak30, MenPeak180                     string
	MemAvg30, MenAvg180                       string
	DiskUsage30, DiskUsage180                 string
	SecurityGroup                             string // CMDB 属性 security_group：关联安全组（名称或 ID，顿号分隔）
}

// joinSecurityGroupsForCMDB 将云上 repeated 安全组名/ID 去重后写入 CMDB 属性 security_group（顿号分隔，与控制台列表习惯一致）。
func joinSecurityGroupsForCMDB(names []string) string {
	if len(names) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(names))
	var parts []string
	for _, raw := range names {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "、")
}

// hostStorageAttr 生成主机 CI 的 storage 属性：系统盘/数据盘任一存在即写入；皆有则为「系统盘:xG&数据盘:yG」。
func hostStorageAttr(systemDiskGB, dataDiskTotalGB int32) string {
	var parts []string
	if systemDiskGB > 0 {
		parts = append(parts, fmt.Sprintf("系统盘:%dG", systemDiskGB))
	}
	if dataDiskTotalGB > 0 {
		parts = append(parts, fmt.Sprintf("数据盘:%dG", dataDiskTotalGB))
	}
	return strings.Join(parts, "&")
}

func buildHosts(resp *pbecs.ListResp, huaweiEcsIDToCceUID map[string]string) []hostRec {
	if resp == nil {
		return nil
	}
	var out []hostRec
	for _, e := range resp.Ecses {
		ip := firstIP(e.GetInnerIps(), e.GetPublicIps())
		memGB := memGBStringFromMB(e.GetMemory())
		cceID := ""
		if e.GetProvider() == pbtenant.CloudProvider_huawei && huaweiEcsIDToCceUID != nil {
			cceID = huaweiEcsIDToCceUID[e.GetInstanceId()]
		}
		util := e.GetUtilizationAudit()
		sysGB := e.GetSystemDiskSizeGb()
		dataGB := e.GetDataDiskTotalGb()
		var diskStr string
		if sysGB > 0 || dataGB > 0 {
			diskStr = fmt.Sprintf("%d+%d", sysGB, dataGB)
		} else {
			diskStr = strings.TrimSpace(e.GetDiskSummary())
		}
		storageAttr := hostStorageAttr(sysGB, dataGB)
		out = append(out, hostRec{
			InstanceID:   strings.TrimSpace(e.GetInstanceId()),
			Name:         e.GetInstanceName(),
			IP:           ip,
			CloudLabel:   cloudTypeLabel(e.GetProvider()),
			Region:       e.GetRegionName(),
			SysNodeName:  effectiveSysNodeName(e.GetProvider(), e.GetRegionName(), e.GetNodeTagValue()),
			CPU:          e.GetCpu(),
			MemGBStr:     memGB,
			DiskStr:      diskStr,
			StorageAttr:  storageAttr,
			OS:           firstNonEmpty(e.GetImageName(), e.GetOsType()),
			CceClusterID: cceID, // 华为：由 CCE ListNodes 的 status.serverId 与集群 metadata.uid 映射得到，与 CceCluster.cluster_uid 一致
			EnvTagValue:  strings.TrimSpace(e.GetEnvTagValue()),
			CpuPeak30:     utilWindowPeakPercentDecimal(util.GetCpuLast_30D()),
			CpuPeak180:    utilWindowPeakPercentDecimal(util.GetCpuLast_180D()),
			CpuAvg30:      utilWindowAvgPercentDecimal(util.GetCpuLast_30D()),
			CpuAvg180:     utilWindowAvgPercentDecimal(util.GetCpuLast_180D()),
			MemPeak30:     utilWindowPeakPercentDecimal(util.GetMemLast_30D()),
			MenPeak180:    utilWindowPeakPercentDecimal(util.GetMemLast_180D()),
			MemAvg30:      utilWindowAvgPercentDecimal(util.GetMemLast_30D()),
			MenAvg180:     utilWindowAvgPercentDecimal(util.GetMemLast_180D()),
			DiskUsage30:   periodUtilizationText(util.GetDiskLast_30D()),
			DiskUsage180:  periodUtilizationText(util.GetDiskLast_180D()),
			SecurityGroup: joinSecurityGroupsForCMDB(e.GetSecurityGroupNames()),
		})
	}
	return out
}

type mwRec struct {
	InstanceID                                                         string // 云实例 ID（可能为非标准字符串；CMDB 写入 instance_id 属性，uuid 见 middlewareCMDBUUID）
	Name, MwType, IP, CPU, Mem, CloudLabel, Region, SysNodeName, DiskStr string
	MiddlewareVersion                                                  string // RDS：引擎+引擎版本；DCS：spec_code；同步至 CMDB middleware_version
	// 以下为利用率百分比字符串（两位小数或空），写入 CMDB 浮点属性时用 cmdbFloatMetricJSON。
	CpuPeak30, CpuAvg30, CpuPeak180, CpuAvg180                         string
	MemPeak30, MenAvg30, MenPeak180, MenAvg180                         string
	SecurityGroup                                                      string // CMDB 属性 security_group
}

// middlewareVersionFromRDS 使用 RDS 的数据库引擎与引擎版本（engine / engine_version）。
func middlewareVersionFromRDS(engine, engineVersion string) string {
	engine = strings.TrimSpace(engine)
	engineVersion = strings.TrimSpace(engineVersion)
	switch {
	case engine != "" && engineVersion != "":
		return engine + " " + engineVersion
	case engineVersion != "":
		return engineVersion
	default:
		return engine
	}
}

func buildMiddlewares(rdsResp *pbrds.ListResp, redisResp *pbredis.ListResp, kafkaResp *pbkafka.ListResp) []mwRec {
	var out []mwRec
	if rdsResp != nil {
		for _, r := range rdsResp.Rdses {
			ip := firstIP(r.GetPrivateIps(), r.GetPublicIps())
			util := r.GetUtilizationAudit()
			out = append(out, mwRec{
				InstanceID:        strings.TrimSpace(r.GetInstanceId()),
				Name:              r.GetInstanceName(),
				MwType:            "RDS_INS",
				IP:                ip,
				CPU:               itoa32(r.GetCpu()),
				Mem:               memGBStringFromMB(r.GetMemoryMb()),
				CloudLabel:        cloudTypeLabel(r.GetProvider()),
				Region:            r.GetRegionName(),
				SysNodeName:       effectiveSysNodeName(r.GetProvider(), r.GetRegionName(), r.GetNodeTagValue()),
				DiskStr:           "", // 列表 API 无独立磁盘字段
				MiddlewareVersion: middlewareVersionFromRDS(r.GetEngine(), r.GetEngineVersion()),
				CpuPeak30:         utilWindowPeakPercentDecimal(util.GetCpuLast_30D()),
				CpuAvg30:          utilWindowAvgPercentDecimal(util.GetCpuLast_30D()),
				CpuPeak180:        utilWindowPeakPercentDecimal(util.GetCpuLast_180D()),
				CpuAvg180:         utilWindowAvgPercentDecimal(util.GetCpuLast_180D()),
				MemPeak30:         utilWindowPeakPercentDecimal(util.GetMemLast_30D()),
				MenAvg30:          utilWindowAvgPercentDecimal(util.GetMemLast_30D()),
				MenPeak180:        utilWindowPeakPercentDecimal(util.GetMemLast_180D()),
				MenAvg180:         utilWindowAvgPercentDecimal(util.GetMemLast_180D()),
				SecurityGroup:     joinSecurityGroupsForCMDB(r.GetSecurityGroupNames()),
			})
		}
	}
	if redisResp != nil {
		for _, r := range redisResp.Redises {
			ip := firstIP(r.GetPrivateIps(), r.GetPublicIps())
			memUtil := r.GetMemoryUtilizationAudit()
			out = append(out, mwRec{
				InstanceID:        strings.TrimSpace(r.GetInstanceId()),
				Name:              r.GetInstanceName(),
				MwType:            "DCS_REDIS",
				IP:                ip,
				CPU:               itoa32(r.GetCpu()),
				Mem:               memGBStringFromMB(r.GetSize()), // 华为 DCS size 为 MB，与 cmdb 展示一致按 GB 字符串
				CloudLabel:        cloudTypeLabel(r.GetProvider()),
				Region:            r.GetRegionName(),
				SysNodeName:       effectiveSysNodeName(r.GetProvider(), r.GetRegionName(), r.GetNodeTagValue()),
				DiskStr:           itoa32(r.GetSize()) + "MB", // 容量规格（与内存 GB 独立对比）
				MiddlewareVersion: strings.TrimSpace(r.GetSpecCode()),
				CpuPeak30:         "",
				CpuAvg30:          "",
				CpuPeak180:        "",
				CpuAvg180:         "",
				MemPeak30:         utilWindowPeakPercentDecimal(memUtil.GetMemLast_30D()),
				MenAvg30:          utilWindowAvgPercentDecimal(memUtil.GetMemLast_30D()),
				MenPeak180:        utilWindowPeakPercentDecimal(memUtil.GetMemLast_180D()),
				MenAvg180:         utilWindowAvgPercentDecimal(memUtil.GetMemLast_180D()),
				SecurityGroup:     joinSecurityGroupsForCMDB(r.GetSecurityGroupNames()),
			})
		}
	}
	if kafkaResp != nil {
		for _, k := range kafkaResp.Kafkas {
			out = append(out, mwRec{
				InstanceID:    strings.TrimSpace(k.GetInstanceId()),
				Name:          k.GetInstanceName(),
				MwType:        "DMS_ROCKETMQ",
				IP:            strings.TrimSpace(k.GetEndPoint()),
				CPU:           "",
				Mem:           "",
				CloudLabel:    cloudTypeLabel(k.GetProvider()),
				Region:        k.GetRegionName(),
				SysNodeName:   effectiveSysNodeName(k.GetProvider(), k.GetRegionName(), k.GetNodeTagValue()),
				DiskStr:       itoa32(k.GetDistSize()) + "GB",
				CpuPeak30:     "",
				CpuAvg30:      "",
				CpuPeak180:    "",
				CpuAvg180:     "",
				MemPeak30:     "",
				MenAvg30:      "",
				MenPeak180:    "",
				MenAvg180:     "",
				SecurityGroup: joinSecurityGroupsForCMDB(k.GetSecurityGroupNames()),
			})
		}
	}
	return out
}

// hostBelongsK8S 对齐 Python YJSCMDBAPI.host_belongs_k8s；需要 ECS 上 CCE 集群 ID 与 CCE 列表 id/uid 一致时才能填 host_ip_new。
func hostBelongsK8S(k8s []k8sCluster, hosts []hostRec) map[string][]string {
	idToName := make(map[string]string)
	for _, c := range k8s {
		if c.ClusterUID != "" {
			idToName[c.ClusterUID] = c.Name
		}
	}
	out := make(map[string][]string)
	for _, h := range hosts {
		if h.CceClusterID == "" {
			continue
		}
		name, ok := idToName[h.CceClusterID]
		if !ok {
			continue
		}
		out[name] = append(out[name], h.IP)
	}
	return out
}

// cmdbCIAdminName CMDB「CI管理员」对应属性 admin_name；默认 admin，可通过 CLOUD_FITTER_CMDB_ADMIN_NAME 覆盖。
func cmdbCIAdminName() string {
	v := strings.TrimSpace(os.Getenv("CLOUD_FITTER_CMDB_ADMIN_NAME"))
	if v == "" {
		return "admin"
	}
	return v
}

func (s *Syncer) addCMDBSystemNodes(systemID, systemName string, nodes map[string]struct{}) componentSyncStats {
	st := componentSyncStats{}
	adminName := cmdbCIAdminName()
	for node := range nodes {
		q := map[string]any{
			"q": fmt.Sprintf("_type:system_node,sys_node_name:%s,system_id:%s", node, systemID),
		}
		exists, err := s.Client.GetCIID(q)
		if err != nil {
			glog.Errorf("cmdb sync system_node(get): system_id=%s node=%q err=%v", systemID, node, err)
			st.Errors++
			continue
		}
		if exists != "" {
			row, err := s.Client.GetCIFirst(q)
			if err != nil {
				glog.Errorf("cmdb sync system_node(get row): system_id=%s node=%q err=%v", systemID, node, err)
				st.Errors++
				continue
			}
			if row != nil && strings.TrimSpace(anyToCompareStr(row["admin_name"])) == adminName {
				glog.Infof("cmdb sync system_node(skip): system_id=%s node=%q id=%s", systemID, node, exists)
				st.Skipped++
				continue
			}
			ciType := "system_node"
			if row != nil {
				if t := strings.TrimSpace(fmt.Sprint(row["ci_type"])); t != "" {
					ciType = t
				} else if t := strings.TrimSpace(fmt.Sprint(row["_type"])); t != "" {
					ciType = t
				}
			}
			_, err = s.Client.UpdateCI(exists, map[string]any{
				"ci_type":    ciType,
				"admin_name": adminName,
			})
			if err != nil {
				glog.Errorf("cmdb sync system_node(update admin_name): system_id=%s node=%q id=%s err=%v", systemID, node, exists, err)
				st.Errors++
				continue
			}
			glog.Infof("cmdb sync system_node(update ok admin_name): system_id=%s node=%q id=%s", systemID, node, exists)
			st.Updated++
			continue
		}
		root, err := s.Client.GetCIID(map[string]any{
			"q": fmt.Sprintf("_type:system,system_id:%s", systemID),
		})
		if err != nil || root == "" {
			glog.Errorf("cmdb sync system_node(root): system_id=%s node=%q err=%v", systemID, node, err)
			st.Errors++
			continue
		}
		rel, err := s.Client.GetSystemLevelRelations(map[string]any{
			"root_id": root,
			"level":   "1,2,3",
			"reverse": 1,
		})
		if err != nil {
			glog.Errorf("cmdb sync system_node(relations): system_id=%s node=%q err=%v", systemID, node, err)
			st.Errors++
			continue
		}
		var bizDomain, productLine, subProduct string
		res, _ := rel["result"].([]any)
		for _, it := range res {
			item, _ := it.(map[string]any)
			if item == nil {
				continue
			}
			switch item["ci_type"] {
			case "biz_domain":
				bizDomain = fmt.Sprint(item["biz_domain_name"])
			case "product_line":
				productLine = fmt.Sprint(item["product_line_name"])
			case "product":
				subProduct = fmt.Sprint(item["product_name"])
			}
		}
		if len(res) == 0 {
			glog.Errorf("cmdb sync system_node(relations): system_id=%s node=%q no relations root=%s", systemID, node, root)
			st.Errors++
			continue
		}
		cloud, loc := splitSysNodeNameForCMDB(node)
		payload := map[string]any{
			"uuid":              uuid.NewString(),
			"ci_type":           "system_node",
			"sys_node_name":     node,
			"system_id":         systemID,
			"system_name":       systemName,
			"product_name":      subProduct,
			"product_line_name": productLine,
			"biz_domain_name":   bizDomain,
			"cloud_type":        cloud,
			"location":          loc,
			"admin_name":        adminName,
		}
		d, err := s.Client.AddCI(payload)
		if err != nil {
			glog.Errorf("cmdb sync system_node(add): system_id=%s node=%q err=%v", systemID, node, err)
			st.Errors++
			continue
		}
		glog.Infof("cmdb sync system_node(add ok): system_id=%s node=%q resp=%+v", systemID, node, d)
		st.Added++
	}
	return st
}

// k8sClusterCMDBFields 与 CMDB K8Sci（ci_type=k8s_cluster）属性对齐：uuid 与 k8s_uuid 均为集群资源 UUID。
func k8sClusterCMDBFields(systemID string, c k8sCluster, clusterUUID, sysNode, ips, ct, locn string) map[string]any {
	return map[string]any{
		"uuid":              clusterUUID,
		"k8s_uuid":          clusterUUID,
		"ci_type":           "k8s_cluster",
		"system_id":         systemID,
		"sys_node_name":     sysNode,
		"k8s_cluster_name":  c.Name,
		"k8s_version":       c.Version,
		"location":          locn,
		"cloud_type":        ct,
		"host_ip_new":       ips,
		"environment":       c.Environment,
		"remarks":           c.Remarks,
	}
}

func (s *Syncer) addCMDBK8sClusters(systemID string, k8s []k8sCluster, clusterToECS map[string][]string) componentSyncStats {
	st := componentSyncStats{}
	for _, c := range k8s {
		if c.Name == "" {
			continue
		}
		sysNode := c.SysNodeName
		if sysNode == "" {
			continue
		}
		clusterUUID, ok := parseCloudInstanceUUID(c.ClusterUID)
		if !ok {
			glog.Warningf("cmdb sync k8s(skip invalid cluster uuid): system_id=%s cluster=%q cluster_uid=%q",
				systemID, c.Name, c.ClusterUID)
			st.Errors++
			continue
		}
		kq := map[string]any{
			"q": fmt.Sprintf("_type:k8s_cluster,uuid:%s,system_id:%s", clusterUUID, systemID),
		}
		exists, err := s.Client.GetCIID(kq)
		if err != nil {
			glog.Errorf("cmdb sync k8s(get): system_id=%s cluster=%q err=%v", systemID, c.Name, err)
			st.Errors++
			continue
		}
		ips := strings.Join(clusterToECS[c.Name], ",")
		ct, locn := cmdbCloudLocationFromSysNodeName(sysNode, c.CloudLabel, c.Region)
		fields := k8sClusterCMDBFields(systemID, c, clusterUUID, sysNode, ips, ct, locn)
		if exists != "" {
			row, err := s.Client.GetCIFirst(kq)
			if err != nil {
				glog.Errorf("cmdb sync k8s(get row): system_id=%s cluster=%q id=%s err=%v", systemID, c.Name, exists, err)
				st.Errors++
				continue
			}
			if row != nil && !k8sResourceChanged(row, clusterUUID, c, ips, ct, locn) {
				glog.Infof("cmdb sync k8s(skip): system_id=%s cluster=%q id=%s", systemID, c.Name, exists)
				st.Skipped++
				continue
			}
			_, err = s.Client.UpdateCI(exists, fields)
			if err != nil {
				glog.Errorf("cmdb sync k8s(update): system_id=%s cluster=%q id=%s err=%v", systemID, c.Name, exists, err)
				st.Errors++
				continue
			}
			glog.Infof("cmdb sync k8s(update ok): system_id=%s cluster=%q id=%s", systemID, c.Name, exists)
			st.Updated++
			continue
		}
		d, err := s.Client.AddCI(fields)
		if err != nil {
			glog.Errorf("cmdb sync k8s(add): system_id=%s cluster=%q err=%v", systemID, c.Name, err)
			st.Errors++
			continue
		}
		glog.Infof("cmdb sync k8s(add ok): system_id=%s cluster=%q resp=%+v", systemID, c.Name, d)
		st.Added++
	}
	return st
}

func (s *Syncer) addCMDBHosts(systemID string, hosts []hostRec) componentSyncStats {
	st := componentSyncStats{}
	for _, h := range hosts {
		if h.IP == "" {
			continue
		}
		sysNode := h.SysNodeName
		if sysNode == "" {
			continue
		}
		instUUID, ok := parseCloudInstanceUUID(h.InstanceID)
		if !ok {
			glog.Warningf("cmdb sync host(skip invalid instance uuid): system_id=%s server_name=%q instance_id=%q",
				systemID, h.Name, h.InstanceID)
			st.Errors++
			continue
		}
		q := map[string]any{
			"q": fmt.Sprintf("_type:server,uuid:%s,system_id:%s", instUUID, systemID),
		}
		exists, err := s.Client.GetCIID(q)
		if err != nil {
			glog.Errorf("cmdb sync host(get): system_id=%s host=%q ip=%s err=%v", systemID, h.Name, h.IP, err)
			st.Errors++
			continue
		}
		if exists != "" {
			row, err := s.Client.GetCIFirst(q)
			if err != nil {
				glog.Errorf("cmdb sync host(get row): system_id=%s host=%q ip=%s id=%s err=%v", systemID, h.Name, h.IP, exists, err)
				st.Errors++
				continue
			}
			if row != nil && !serverResourceChanged(row, h) {
				glog.Infof("cmdb sync host(skip): system_id=%s host=%q ip=%s id=%s", systemID, h.Name, h.IP, exists)
				st.Skipped++
				continue
			}
			ciType := "server"
			if row != nil {
				if t := strings.TrimSpace(fmt.Sprint(row["ci_type"])); t != "" {
					ciType = t
				} else if t := strings.TrimSpace(fmt.Sprint(row["_type"])); t != "" {
					ciType = t
				}
			}
			_, err = s.Client.UpdateCI(exists, map[string]any{
				"ci_type":        ciType,
				"cpu_count":      int(h.CPU),
				"ram_size":       h.MemGBStr,
				"disk_size":      h.DiskStr,
				"storage":        h.StorageAttr,
				"cpu_peak_30":    cmdbFloatMetricJSON(h.CpuPeak30),
				"cpu_peak_180":   cmdbFloatMetricJSON(h.CpuPeak180),
				"cpu_avg_30":     cmdbFloatMetricJSON(h.CpuAvg30),
				"cpu_avg_180":    cmdbFloatMetricJSON(h.CpuAvg180),
				"mem_peak_30":    cmdbFloatMetricJSON(h.MemPeak30),
				"men_peak_180":   cmdbFloatMetricJSON(h.MenPeak180),
				"men_avg_30":     cmdbFloatMetricJSON(h.MemAvg30),
				"men_avg_180":    cmdbFloatMetricJSON(h.MenAvg180),
				"disk_usage_30":  cmdbFloatMetricJSON(h.DiskUsage30),
				"disk_usage_180": cmdbFloatMetricJSON(h.DiskUsage180),
				"security_group": h.SecurityGroup,
			})
			if err != nil {
				glog.Errorf("cmdb sync host(update): system_id=%s host=%q ip=%s id=%s err=%v", systemID, h.Name, h.IP, exists, err)
				st.Errors++
				continue
			}
			glog.Infof("cmdb sync host(update ok): system_id=%s host=%q ip=%s id=%s", systemID, h.Name, h.IP, exists)
			st.Updated++
			continue
		}
		ct, locn := cmdbCloudLocationFromSysNodeName(sysNode, h.CloudLabel, h.Region)
		payload := map[string]any{
			"uuid":           instUUID,
			"ci_type":        "server",
			"system_id":      systemID,
			"sys_node_name":  sysNode,
			"server_name":    h.Name,
			"private_ip":     h.IP,
			"cpu_count":      int(h.CPU),
			"ram_size":       h.MemGBStr,
			"disk_size":      h.DiskStr,
			"storage":        h.StorageAttr,
			"os_version":     h.OS,
			"location":       locn,
			"cloud_type":     ct,
			"cpu_peak_30":    cmdbFloatMetricJSON(h.CpuPeak30),
			"cpu_peak_180":   cmdbFloatMetricJSON(h.CpuPeak180),
			"cpu_avg_30":     cmdbFloatMetricJSON(h.CpuAvg30),
			"cpu_avg_180":    cmdbFloatMetricJSON(h.CpuAvg180),
			"mem_peak_30":    cmdbFloatMetricJSON(h.MemPeak30),
			"men_peak_180":   cmdbFloatMetricJSON(h.MenPeak180),
			"men_avg_30":     cmdbFloatMetricJSON(h.MemAvg30),
			"men_avg_180":    cmdbFloatMetricJSON(h.MenAvg180),
			"disk_usage_30":  cmdbFloatMetricJSON(h.DiskUsage30),
			"disk_usage_180": cmdbFloatMetricJSON(h.DiskUsage180),
			"security_group": h.SecurityGroup,
		}
		d, err := s.Client.AddCI(payload)
		if err != nil {
			glog.Errorf("cmdb sync host(add): system_id=%s host=%q ip=%s err=%v", systemID, h.Name, h.IP, err)
			st.Errors++
			continue
		}
		glog.Infof("cmdb sync host(add ok): system_id=%s host=%q ip=%s resp=%+v", systemID, h.Name, h.IP, d)
		st.Added++
	}
	return st
}

func (s *Syncer) addCMDBMiddlewares(systemID string, mws []mwRec) componentSyncStats {
	st := componentSyncStats{}
	for _, m := range mws {
		if m.Name == "" {
			continue
		}
		sysNode := m.SysNodeName
		if sysNode == "" {
			continue
		}
		rawID := strings.TrimSpace(m.InstanceID)
		if rawID == "" {
			glog.Warningf("cmdb sync middleware(skip empty instance_id): system_id=%s name=%q", systemID, m.Name)
			st.Errors++
			continue
		}
		instUUID, ok := middlewareCMDBUUID(rawID)
		if !ok {
			glog.Warningf("cmdb sync middleware(skip instance_id): system_id=%s name=%q instance_id=%q", systemID, m.Name, rawID)
			st.Errors++
			continue
		}
		// 唯一性对齐：优先 instance_id + system_id；旧数据仅有 uuid 时回退按 uuid 命中并补写 instance_id
		mqInst := map[string]any{"q": fmt.Sprintf("_type:middle_software,instance_id:%s,system_id:%s", rawID, systemID)}
		exists, err := s.Client.GetCIID(mqInst)
		if err != nil {
			glog.Errorf("cmdb sync middleware(get by instance_id): system_id=%s name=%q err=%v", systemID, m.Name, err)
			st.Errors++
			continue
		}
		var row map[string]any
		if exists != "" {
			row, err = s.Client.GetCIFirst(mqInst)
			if err != nil {
				glog.Errorf("cmdb sync middleware(get row by instance_id): system_id=%s name=%q id=%s err=%v", systemID, m.Name, exists, err)
				st.Errors++
				continue
			}
		} else {
			mqUUID := map[string]any{"q": fmt.Sprintf("_type:middle_software,uuid:%s,system_id:%s", instUUID, systemID)}
			exists, err = s.Client.GetCIID(mqUUID)
			if err != nil {
				glog.Errorf("cmdb sync middleware(get by uuid): system_id=%s name=%q err=%v", systemID, m.Name, err)
				st.Errors++
				continue
			}
			if exists != "" {
				row, err = s.Client.GetCIFirst(mqUUID)
				if err != nil {
					glog.Errorf("cmdb sync middleware(get row by uuid): system_id=%s name=%q id=%s err=%v", systemID, m.Name, exists, err)
					st.Errors++
					continue
				}
			}
		}
		if exists != "" {
			if row != nil && !middlewareResourceChanged(row, m) {
				glog.Infof("cmdb sync middleware(skip): system_id=%s name=%q id=%s", systemID, m.Name, exists)
				st.Skipped++
				continue
			}
			ciType := "middle_software"
			if row != nil {
				if t := strings.TrimSpace(fmt.Sprint(row["ci_type"])); t != "" {
					ciType = t
				} else if t := strings.TrimSpace(fmt.Sprint(row["_type"])); t != "" {
					ciType = t
				}
			}
			_, err = s.Client.UpdateCI(exists, map[string]any{
				"uuid":               instUUID,
				"instance_id":        rawID,
				"ci_type":            ciType,
				"cpu_count":          m.CPU,
				"ram_size":           m.Mem,
				"disk_size":          m.DiskStr,
				"cpu_peak_30":        cmdbFloatMetricJSON(m.CpuPeak30),
				"cpu_avg_30":         cmdbFloatMetricJSON(m.CpuAvg30),
				"cpu_peak_180":       cmdbFloatMetricJSON(m.CpuPeak180),
				"cpu_avg_180":        cmdbFloatMetricJSON(m.CpuAvg180),
				"mem_peak_30":        cmdbFloatMetricJSON(m.MemPeak30),
				"men_avg_30":         cmdbFloatMetricJSON(m.MenAvg30),
				"men_peak_180":       cmdbFloatMetricJSON(m.MenPeak180),
				"men_avg_180":        cmdbFloatMetricJSON(m.MenAvg180),
				"sys_node_name":      sysNode,
				"resource_name":      m.Name,
				"resource_type":      m.MwType,
				"location":           m.CloudLabel,
				"cloud_type":         m.Region,
				"private_ip":         m.IP,
				"middleware_version": m.MiddlewareVersion,
				"security_group":     m.SecurityGroup,
			})
			if err != nil {
				glog.Errorf("cmdb sync middleware(update): system_id=%s name=%q id=%s err=%v", systemID, m.Name, exists, err)
				st.Errors++
				continue
			}
			glog.Infof("cmdb sync middleware(update ok): system_id=%s name=%q id=%s", systemID, m.Name, exists)
			st.Updated++
			continue
		}
		// 与 cmdb_api.py 中 add_cmdb_middlewares 字段一致（含 location / cloud_type 与 Python 相同赋值方式）
		payload := map[string]any{
			"uuid":               instUUID,
			"instance_id":        rawID,
			"ci_type":            "middle_software",
			"system_id":          systemID,
			"sys_node_name":      sysNode,
			"resource_name":      m.Name,
			"resource_type":      m.MwType,
			"location":           m.CloudLabel,
			"cloud_type":         m.Region,
			"private_ip":         m.IP,
			"cpu_count":          m.CPU,
			"ram_size":           m.Mem,
			"disk_size":          m.DiskStr,
			"cpu_peak_30":        cmdbFloatMetricJSON(m.CpuPeak30),
			"cpu_avg_30":         cmdbFloatMetricJSON(m.CpuAvg30),
			"cpu_peak_180":       cmdbFloatMetricJSON(m.CpuPeak180),
			"cpu_avg_180":        cmdbFloatMetricJSON(m.CpuAvg180),
			"mem_peak_30":        cmdbFloatMetricJSON(m.MemPeak30),
			"men_avg_30":         cmdbFloatMetricJSON(m.MenAvg30),
			"men_peak_180":       cmdbFloatMetricJSON(m.MenPeak180),
			"men_avg_180":        cmdbFloatMetricJSON(m.MenAvg180),
			"middleware_version": m.MiddlewareVersion,
			"security_group":     m.SecurityGroup,
		}
		d, err := s.Client.AddCI(payload)
		if err != nil {
			glog.Errorf("cmdb sync middleware(add): system_id=%s name=%q err=%v", systemID, m.Name, err)
			st.Errors++
			continue
		}
		glog.Infof("cmdb sync middleware(add ok): system_id=%s name=%q resp=%+v", systemID, m.Name, d)
		st.Added++
	}
	return st
}

func mergeEIPSystemNodeKeys(nodes map[string]struct{}, eips []*eip.Instance) {
	for _, e := range eips {
		if e == nil {
			continue
		}
		k := effectiveSysNodeName(eipTenantProvider(e), strings.TrimSpace(e.RegionName), "")
		if k != "" {
			nodes[k] = struct{}{}
		}
	}
}

// mergeELBSystemNodeKeys 与 EIP 一致：节点名为 effectiveSysNodeName(云, 地域, "")，与 ECS 默认节点命名规则对齐。
func mergeELBSystemNodeKeys(nodes map[string]struct{}, elbs []*elb.Instance) {
	for _, e := range elbs {
		if e == nil {
			continue
		}
		k := effectiveSysNodeName(elbTenantProvider(e), strings.TrimSpace(e.RegionName), "")
		if k != "" {
			nodes[k] = struct{}{}
		}
	}
}

func elbTenantProvider(inst *elb.Instance) pbtenant.CloudProvider {
	switch strings.ToLower(strings.TrimSpace(inst.Provider)) {
	case "huawei":
		return pbtenant.CloudProvider_huawei
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

func eipTenantProvider(inst *eip.Instance) pbtenant.CloudProvider {
	switch strings.ToLower(strings.TrimSpace(inst.Provider)) {
	case "huawei":
		return pbtenant.CloudProvider_huawei
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

func mergeAttrMaps(base map[string]any, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func eipCIChanged(row map[string]any, want map[string]any) bool {
	for k, v := range want {
		if strings.TrimSpace(anyToCompareStr(row[k])) != strings.TrimSpace(anyToCompareStr(v)) {
			return true
		}
	}
	return false
}

// cmdbSyncEIPBandwidthType 同步至 CMDB 的 bandwidth_type（模型侧多为短文本）：华为侧 PER（独享带宽）写「独享」，其余写「共享」；不同步英文 PER/WHOLE，避免多 EIP 同写 PER 触发 CMDB 错误唯一约束。
// 可选环境变量覆盖展示文案：CLOUD_FITTER_CMDB_EIP_BW_EXCLUSIVE、CLOUD_FITTER_CMDB_EIP_BW_SHARED。
func cmdbSyncEIPBandwidthType(raw string) string {
	exclusive := strings.TrimSpace(os.Getenv("CLOUD_FITTER_CMDB_EIP_BW_EXCLUSIVE"))
	if exclusive == "" {
		exclusive = "独享"
	}
	shared := strings.TrimSpace(os.Getenv("CLOUD_FITTER_CMDB_EIP_BW_SHARED"))
	if shared == "" {
		shared = "共享"
	}
	r := strings.TrimSpace(raw)
	switch {
	case r == "&{PER}", strings.EqualFold(r, "PER"):
		return exclusive
	default:
		return shared
	}
}

// cmdbSyncEIPStatus 同步至 CMDB 的 eip_status（不再使用属性名 status）。
// CMDB 常为下拉枚举：写入「激活」等中文若未在模型白名单会返回 400。
// 默认写入 ACTIVE / DOWN（与华为 API 枚举一致）；若要同步中文，请在 CMDB 模型中增加可选值后设置：
// CLOUD_FITTER_CMDB_EIP_STATUS_ACTIVE、CLOUD_FITTER_CMDB_EIP_STATUS_OTHER。
func cmdbSyncEIPStatus(raw string) string {
	active := strings.TrimSpace(os.Getenv("CLOUD_FITTER_CMDB_EIP_STATUS_ACTIVE"))
	if active == "" {
		active = "ACTIVE"
	}
	other := strings.TrimSpace(os.Getenv("CLOUD_FITTER_CMDB_EIP_STATUS_OTHER"))
	if other == "" {
		other = "DOWN"
	}
	r := strings.TrimSpace(raw)
	switch {
	case r == "&{ACTIVE}", strings.EqualFold(r, "ACTIVE"):
		return active
	default:
		return other
	}
}

func (s *Syncer) addCMDBEIPs(systemID string, eips []*eip.Instance) componentSyncStats {
	st := componentSyncStats{}
	for _, e := range eips {
		if e == nil || strings.TrimSpace(e.EipId) == "" {
			continue
		}
		eipUUID, ok := parseCloudInstanceUUID(e.EipId)
		if !ok {
			glog.Warningf("cmdb sync eip(skip invalid instance uuid): system_id=%s eip_id=%q", systemID, e.EipId)
			st.Errors++
			continue
		}
		sysNode := effectiveSysNodeName(eipTenantProvider(e), strings.TrimSpace(e.RegionName), "")
		if sysNode == "" {
			glog.Warningf("cmdb sync eip(skip no sys_node): system_id=%s eip_id=%s", systemID, e.EipId)
			st.Errors++
			continue
		}
		q := map[string]any{
			"q": fmt.Sprintf("_type:EIP,uuid:%s,system_id:%s", eipUUID, systemID),
		}
		exists, err := s.Client.GetCIID(q)
		if err != nil {
			glog.Errorf("cmdb sync eip(get): system_id=%s eip_id=%s err=%v", systemID, e.EipId, err)
			st.Errors++
			continue
		}
		fields := map[string]any{
			"eip_ip":              strings.TrimSpace(e.Eip),
			"bandwidth_type":      cmdbSyncEIPBandwidthType(e.BandwidthType),
			"bandwidth":           strconv.FormatInt(int64(e.BandwidthSizeMbit), 10),
			"private_ip":          strings.TrimSpace(e.PrivateIpAddress),
			"eip_status":          cmdbSyncEIPStatus(e.Status),
			"bound_resource_type": strings.TrimSpace(e.BindInstanceType),
			"sys_node_name":       sysNode,
		}
		if exists != "" {
			row, err := s.Client.GetCIFirst(q)
			if err != nil {
				glog.Errorf("cmdb sync eip(get row): system_id=%s eip_id=%s err=%v", systemID, e.EipId, err)
				st.Errors++
				continue
			}
			if row != nil && !eipCIChanged(row, fields) {
				glog.Infof("cmdb sync eip(skip): system_id=%s eip_id=%s id=%s", systemID, e.EipId, exists)
				st.Skipped++
				continue
			}
			_, err = s.Client.UpdateCI(exists, mergeAttrMaps(map[string]any{"ci_type": "EIP"}, fields))
			if err != nil {
				glog.Errorf("cmdb sync eip(update): system_id=%s eip_id=%s id=%s err=%v", systemID, e.EipId, exists, err)
				st.Errors++
				continue
			}
			glog.Infof("cmdb sync eip(update ok): system_id=%s eip_id=%s id=%s", systemID, e.EipId, exists)
			st.Updated++
			continue
		}
		payload := mergeAttrMaps(map[string]any{
			"uuid":      eipUUID,
			"ci_type":   "EIP",
			"system_id": systemID,
		}, fields)
		d, err := s.Client.AddCI(payload)
		if err != nil {
			glog.Errorf("cmdb sync eip(add): system_id=%s eip_id=%s err=%v", systemID, e.EipId, err)
			st.Errors++
			continue
		}
		glog.Infof("cmdb sync eip(add ok): system_id=%s eip_id=%s resp=%+v", systemID, e.EipId, d)
		st.Added++
	}
	return st
}

// addCMDBELBs 写入 CI 类型 ELB；唯一查找条件为 uuid（云负载均衡实例 ID，RFC4122）+ system_id。
func (s *Syncer) addCMDBELBs(systemID string, elbs []*elb.Instance) componentSyncStats {
	st := componentSyncStats{}
	for _, e := range elbs {
		if e == nil || strings.TrimSpace(e.ID) == "" {
			continue
		}
		elbUUID, ok := parseCloudInstanceUUID(e.ID)
		if !ok {
			glog.Warningf("cmdb sync elb(skip invalid instance uuid): system_id=%s elb_id=%q", systemID, e.ID)
			st.Errors++
			continue
		}
		elbName := strings.TrimSpace(e.Name)
		sysNode := effectiveSysNodeName(elbTenantProvider(e), strings.TrimSpace(e.RegionName), "")
		if sysNode == "" {
			glog.Warningf("cmdb sync elb(skip no sys_node): system_id=%s elb_id=%s", systemID, e.ID)
			st.Errors++
			continue
		}
		q := map[string]any{
			"q": fmt.Sprintf("_type:ELB,uuid:%s,system_id:%s", elbUUID, systemID),
		}
		exists, err := s.Client.GetCIID(q)
		if err != nil {
			glog.Errorf("cmdb sync elb(get): system_id=%s elb_id=%s err=%v", systemID, e.ID, err)
			st.Errors++
			continue
		}
		fields := map[string]any{
			"uuid":                   elbUUID,
			"elb_name":               elbName,
			"listener_protocol_port": strings.TrimSpace(e.Listeners),
			"ipv4_private_address":   strings.TrimSpace(e.IPv4Private),
			"ipv4_public_address":    strings.TrimSpace(e.IPv4Public),
			"ipv4_bandwidth":         strconv.FormatInt(int64(e.IPv4BandwidthMbit), 10),
			"sys_node_name":          sysNode,
		}
		if exists != "" {
			row, err := s.Client.GetCIFirst(q)
			if err != nil {
				glog.Errorf("cmdb sync elb(get row): system_id=%s elb_id=%s err=%v", systemID, e.ID, err)
				st.Errors++
				continue
			}
			if row != nil && !eipCIChanged(row, fields) {
				glog.Infof("cmdb sync elb(skip): system_id=%s elb_id=%s id=%s", systemID, e.ID, exists)
				st.Skipped++
				continue
			}
			_, err = s.Client.UpdateCI(exists, mergeAttrMaps(map[string]any{"ci_type": "ELB"}, fields))
			if err != nil {
				glog.Errorf("cmdb sync elb(update): system_id=%s elb_id=%s id=%s err=%v", systemID, e.ID, exists, err)
				st.Errors++
				continue
			}
			glog.Infof("cmdb sync elb(update ok): system_id=%s elb_id=%s id=%s", systemID, e.ID, exists)
			st.Updated++
			continue
		}
		payload := mergeAttrMaps(map[string]any{
			"ci_type":   "ELB",
			"system_id": systemID,
		}, fields)
		d, err := s.Client.AddCI(payload)
		if err != nil {
			glog.Errorf("cmdb sync elb(add): system_id=%s elb_id=%s err=%v", systemID, e.ID, err)
			st.Errors++
			continue
		}
		glog.Infof("cmdb sync elb(add ok): system_id=%s elb_id=%s resp=%+v", systemID, e.ID, d)
		st.Added++
	}
	return st
}

func billingCostFieldsChanged(row map[string]any, rowCount, totalCost, resourceCategory string) bool {
	if strings.TrimSpace(anyToCompareStr(row["resource_category"])) != strings.TrimSpace(resourceCategory) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["row_count"])) != strings.TrimSpace(rowCount) {
		return true
	}
	if !metricStrEqual(strings.TrimSpace(anyToCompareStr(row["total_cost"])), strings.TrimSpace(totalCost)) {
		return true
	}
	return false
}

// addCMDBBillings 将消费大类汇总写入 CMDB 模型 billing。
//
// 关联关系按「系统」维护：必需字段 system_id（与 CMDB system、本地系统 system_id 对齐），不再使用节点名称；
// 同一系统下按 billing_month、resource_category、account_name（云账号）唯一标识一行；
// resource_category 与账单汇总接口大类一致（含 EIP/网络、负载均衡、对象存储、VPC 等，见 billingagg）。
// 消费合计按两位小数四舍五入后为 0 的行不同步（与前端「—」一致）：不新增、不更新；若 CMDB 已有同键 billing CI 则删除。
func (s *Syncer) addCMDBBillings(systemID, billingMonth, accountName string, resp *pbbilling.ListBillingSummaryResp) componentSyncStats {
	st := componentSyncStats{}
	if resp == nil {
		return st
	}
	billingMonth = strings.TrimSpace(billingMonth)
	accountName = strings.TrimSpace(accountName)
	if billingMonth == "" || accountName == "" {
		glog.Errorf("cmdb sync billing: empty billing_month or account_name")
		st.Errors++
		return st
	}
	if _, err := time.Parse("2006-01", billingMonth); err != nil {
		glog.Errorf("cmdb sync billing: invalid billing_month=%q: %v", billingMonth, err)
		st.Errors++
		return st
	}
	curDefault := strings.TrimSpace(resp.Currency)
	if curDefault == "" {
		curDefault = "CNY"
	}
	for _, row := range resp.Rows {
		if row == nil {
			continue
		}
		cat := strings.TrimSpace(row.Category)
		if cat == "" {
			continue
		}
		rounded := billingagg.RoundMoney2(row.TotalConsumeAmount)
		if math.IsNaN(rounded) || math.IsInf(rounded, 0) {
			glog.Warningf("cmdb sync billing(skip invalid amount): system_id=%s account=%s category=%q raw=%v", systemID, accountName, cat, row.TotalConsumeAmount)
			st.Skipped++
			continue
		}
		q := map[string]any{
			"q": fmt.Sprintf("_type:billing,system_id:%s,billing_month:%s,resource_category:%s,account_name:%s", systemID, billingMonth, cat, accountName),
		}
		if rounded == 0 {
			exists0, err := s.Client.GetCIID(q)
			if err != nil {
				glog.Errorf("cmdb sync billing(get zero consume): system_id=%s account=%s category=%q err=%v", systemID, accountName, cat, err)
				st.Errors++
				continue
			}
			if exists0 == "" {
				glog.Infof("cmdb sync billing(skip zero consume, no ci): system_id=%s account=%s category=%q", systemID, accountName, cat)
				st.Skipped++
				continue
			}
			if _, err := s.Client.DeleteCI(exists0); err != nil {
				glog.Errorf("cmdb sync billing(delete zero consume): system_id=%s account=%s category=%q id=%s err=%v", systemID, accountName, cat, exists0, err)
				st.Errors++
				continue
			}
			glog.Infof("cmdb sync billing(delete ok zero consume): system_id=%s account=%s category=%s id=%s", systemID, accountName, cat, exists0)
			st.Deleted++
			continue
		}
		cur := strings.TrimSpace(row.Currency)
		if cur == "" {
			cur = curDefault
		}
		exists, err := s.Client.GetCIID(q)
		if err != nil {
			glog.Errorf("cmdb sync billing(get): system_id=%s account=%s category=%q err=%v", systemID, accountName, cat, err)
			st.Errors++
			continue
		}
		rowCountStr := itoa32(row.SourceRowCount)
		totalStr := strconv.FormatFloat(row.TotalConsumeAmount, 'f', 2, 64)
		fields := map[string]any{
			"currency":           cur,
			"row_count":          rowCountStr,
			"total_cost":         totalStr,
			"account_name":       accountName,
			"resource_category": cat,
		}
		if exists != "" {
			frow, err := s.Client.GetCIFirst(q)
			if err != nil {
				glog.Errorf("cmdb sync billing(get row): system_id=%s account=%s category=%q err=%v", systemID, accountName, cat, err)
				st.Errors++
				continue
			}
			if frow != nil && !billingCostFieldsChanged(frow, rowCountStr, totalStr, cat) {
				glog.Infof("cmdb sync billing(skip): system_id=%s account=%s category=%s id=%s", systemID, accountName, cat, exists)
				st.Skipped++
				continue
			}
			_, err = s.Client.UpdateCI(exists, mergeAttrMaps(map[string]any{"ci_type": "billing"}, fields))
			if err != nil {
				glog.Errorf("cmdb sync billing(update): system_id=%s account=%s category=%q id=%s err=%v", systemID, accountName, cat, exists, err)
				st.Errors++
				continue
			}
			glog.Infof("cmdb sync billing(update ok): system_id=%s account=%s category=%s id=%s", systemID, accountName, cat, exists)
			st.Updated++
			continue
		}
		payload := mergeAttrMaps(map[string]any{
			"uuid":              uuid.NewString(),
			"ci_type":           "billing",
			"system_id":         systemID,
			"billing_month":     billingMonth,
			"resource_category": cat,
		}, fields)
		d, err := s.Client.AddCI(payload)
		if err != nil {
			glog.Errorf("cmdb sync billing(add): system_id=%s account=%s category=%q err=%v", systemID, accountName, cat, err)
			st.Errors++
			continue
		}
		glog.Infof("cmdb sync billing(add ok): system_id=%s account=%s category=%s resp=%+v", systemID, accountName, cat, d)
		st.Added++
	}
	return st
}

func cloudTypeLabel(p pbtenant.CloudProvider) string {
	switch p {
	case pbtenant.CloudProvider_ali:
		return "阿里云"
	case pbtenant.CloudProvider_tencent:
		return "腾讯云"
	case pbtenant.CloudProvider_huawei:
		return "华为云"
	case pbtenant.CloudProvider_aws:
		return "AWS"
	default:
		return "云"
	}
}

func firstIP(a, b []string) string {
	if len(a) > 0 && strings.TrimSpace(a[0]) != "" {
		return strings.TrimSpace(a[0])
	}
	if len(b) > 0 {
		return strings.TrimSpace(b[0])
	}
	return ""
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func memGBStringFromMB(mb int32) string {
	if mb <= 0 {
		return "0"
	}
	g := float64(mb) / 1024.0
	return strconv.FormatFloat(g, 'f', 1, 64)
}

func itoa32(n int32) string {
	if n == 0 {
		return "0"
	}
	return strconv.FormatInt(int64(n), 10)
}

func utilWindowPeakText(w *pbutilization.UtilizationWindow) string {
	if w == nil || !w.GetAvailable() {
		return ""
	}
	return percentIntText(w.GetPeakPercent())
}

// utilWindowPeakPercentDecimal 窗口内峰值百分比（CPU/内存）：保留小数，供主机利用率写入 CMDB 浮点属性。
func utilWindowPeakPercentDecimal(w *pbutilization.UtilizationWindow) string {
	if w == nil || !w.GetAvailable() {
		return ""
	}
	return percentText(w.GetPeakPercent())
}

// utilWindowAvgPercentDecimal 窗口内平均值百分比：保留小数，同上。
func utilWindowAvgPercentDecimal(w *pbutilization.UtilizationWindow) string {
	if w == nil || !w.GetAvailable() {
		return ""
	}
	return percentText(w.GetAvgPercent())
}

func utilWindowAvgText(w *pbutilization.UtilizationWindow) string {
	if w == nil || !w.GetAvailable() {
		return ""
	}
	return percentIntText(w.GetAvgPercent())
}

func periodUtilizationText(w *pbutilization.PeriodUtilizationRate) string {
	if w == nil || !w.GetAvailable() {
		return ""
	}
	return percentText(w.GetUtilizationPercent())
}

func percentText(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func percentIntText(v float64) string {
	return strconv.FormatInt(int64(math.Round(v)), 10)
}

// StartDailyAt 在每天指定本地时刻执行一次 Run（hour/min 为 24 小时制；默认 2,0 即凌晨 02:00，非下午）。
func (s *Syncer) StartDailyAt(hour, min int) {
	if s == nil {
		return
	}
	loc := time.Local
	if v := os.Getenv("TZ"); v != "" {
		if l, err := time.LoadLocation(v); err == nil {
			loc = l
		}
	}
	go func() {
		for {
			now := time.Now().In(loc)
			next := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, loc)
			if !next.After(now) {
				next = next.Add(24 * time.Hour)
			}
			wait := time.Until(next)
			glog.Infof("cmdb sync: next run at %s (in %v)", next.Format(time.RFC3339), wait)
			time.Sleep(wait)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
			if err := s.Run(ctx); err != nil {
				glog.Errorf("cmdb sync run: %v", err)
			}
			cancel()
		}
	}()
}

// CMDBConfigFromEnv 从环境变量读取 CMDB 地址与密钥；三者皆非空则启用。变量名：CLOUD_FITTER_CMDB_BASE_URL, CLOUD_FITTER_CMDB_KEY, CLOUD_FITTER_CMDB_SECRET。
// 云资源快照与「仅从快照同步 CMDB」见 resourcecache 包：CLOUD_FITTER_RESOURCE_SNAPSHOT_*、CLOUD_FITTER_RESOURCE_SNAPSHOT_BLACKOUT_LOCAL、CLOUD_FITTER_CMDB_USE_RESOURCE_SNAPSHOT。
func CMDBConfigFromEnv() (base, key, secret string, ok bool) {
	base = strings.TrimSpace(os.Getenv("CLOUD_FITTER_CMDB_BASE_URL"))
	key = strings.TrimSpace(os.Getenv("CLOUD_FITTER_CMDB_KEY"))
	secret = strings.TrimSpace(os.Getenv("CLOUD_FITTER_CMDB_SECRET"))
	return base, key, secret, base != "" && key != "" && secret != ""
}
