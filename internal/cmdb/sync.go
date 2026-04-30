package cmdb

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbcce"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbecs"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbkafka"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbrds"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbredis"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbutilization"
	"github.com/cloud-fitter/cloud-fitter/internal/configstore"
	"github.com/cloud-fitter/cloud-fitter/internal/server/jsonapi"
)

// Syncer 将 cloud-fitter 拉取逻辑（与 jsonapi 按 systemName 一致）与 cmdb-sync/api/cmdb_api.py 中写入 CMDB 的步骤对齐。
type Syncer struct {
	Client *Client
	Store  *configstore.Store
}

// Run 与 Python main.cmdb 一致：分页拉取 CMDB 中全部 system，再对每个 system_id 同步；云资源来自本服务 List*BySystemName，不再请求 YJSCMDBAPI。
func (s *Syncer) Run(ctx context.Context) error {
	if s == nil || s.Client == nil || s.Store == nil {
		return errors.New("cmdb syncer: nil client or store")
	}
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

	ecsResp, err := jsonapi.ListEcsBySystemName(ctx, systemName)
	if err != nil {
		return errors.Wrap(err, "ListEcsBySystemName")
	}
	rdsResp, err := jsonapi.ListRdsBySystemName(ctx, systemName)
	if err != nil {
		return errors.Wrap(err, "ListRdsBySystemName")
	}
	redisResp, err := jsonapi.ListRedisBySystemName(ctx, systemName)
	if err != nil {
		return errors.Wrap(err, "ListRedisBySystemName")
	}
	kafkaResp, err := jsonapi.ListKafkaBySystemName(ctx, systemName)
	if err != nil {
		return errors.Wrap(err, "ListKafkaBySystemName")
	}
	cceResp, err := jsonapi.ListCceBySystemName(ctx, systemName)
	if err != nil {
		return errors.Wrap(err, "ListCceBySystemName")
	}

	// 华为 CCE：ListClusters + ListNodes 得到 ECS 实例 ID -> 集群 UID，供 host_ip_new
	ecsIDToCceUID, err := huaweiEcsIDToClusterUIDMap(ctx, s.Store, systemName)
	if err != nil {
		glog.Warningf("cmdb sync: huawei ecs->cce uid map: %v (host_ip_new may be empty)", err)
		ecsIDToCceUID = nil
	}

	systemNodes := collectSystemNodeKeys(ecsResp, rdsResp, redisResp, kafkaResp, cceResp)
	k8sList := buildK8sClusters(cceResp)
	hosts := buildHosts(ecsResp, ecsIDToCceUID)
	middlewares := buildMiddlewares(rdsResp, redisResp, kafkaResp)

	clusterToECS := hostBelongsK8S(k8sList, hosts)

	s.addCMDBSystemNodes(systemID, sysRow.Name, systemNodes)
	s.addCMDBK8sClusters(systemID, k8sList, clusterToECS)
	s.addCMDBHosts(systemID, hosts)
	s.addCMDBMiddlewares(systemID, middlewares)
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
}

func buildK8sClusters(resp *pbcce.ListResp) []k8sCluster {
	if resp == nil {
		return nil
	}
	var out []k8sCluster
	for _, c := range resp.Clusters {
		out = append(out, k8sCluster{
			Name:        c.GetClusterName(),
			Version:     c.GetK8SVersion(),
			CloudLabel:  cloudTypeLabel(c.GetProvider()),
			Region:      c.GetRegionName(),
			ClusterUID:  c.GetClusterUid(),
			SysNodeName: effectiveSysNodeName(c.GetProvider(), c.GetRegionName(), c.GetNodeTagValue()),
		})
	}
	return out
}

type hostRec struct {
	Name, IP, CloudLabel, Region, SysNodeName string
	CPU                                       int32
	MemGBStr                                  string
	DiskStr                                   string // 磁盘：优先 disk_summary，否则 系统盘GB+数据盘GB
	OS                                        string
	CceClusterID                              string
	CpuPeak30, CpuPeak180                     string
	CpuAvg30, CpuAvg180                       string
	MemPeak30, MenPeak180                     string
	MemAvg30, MenAvg180                       string
	DiskUsage30, DiskUsage180                 string
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
		diskStr := strings.TrimSpace(e.GetDiskSummary())
		if diskStr == "" {
			diskStr = fmt.Sprintf("%d+%d", e.GetSystemDiskSizeGb(), e.GetDataDiskTotalGb())
		}
		out = append(out, hostRec{
			Name:         e.GetInstanceName(),
			IP:           ip,
			CloudLabel:   cloudTypeLabel(e.GetProvider()),
			Region:       e.GetRegionName(),
			SysNodeName:  effectiveSysNodeName(e.GetProvider(), e.GetRegionName(), e.GetNodeTagValue()),
			CPU:          e.GetCpu(),
			MemGBStr:     memGB,
			DiskStr:      diskStr,
			OS:           firstNonEmpty(e.GetImageName(), e.GetOsType()),
			CceClusterID: cceID, // 华为：由 CCE ListNodes 的 status.serverId 与集群 metadata.uid 映射得到，与 CceCluster.cluster_uid 一致
			CpuPeak30:    utilWindowPeakText(util.GetCpuLast_30D()),
			CpuPeak180:   utilWindowPeakText(util.GetCpuLast_180D()),
			CpuAvg30:     utilWindowAvgText(util.GetCpuLast_30D()),
			CpuAvg180:    utilWindowAvgText(util.GetCpuLast_180D()),
			MemPeak30:    utilWindowPeakText(util.GetMemLast_30D()),
			MenPeak180:   utilWindowPeakText(util.GetMemLast_180D()),
			MemAvg30:     utilWindowAvgText(util.GetMemLast_30D()),
			MenAvg180:    utilWindowAvgText(util.GetMemLast_180D()),
			DiskUsage30:  periodUtilizationText(util.GetDiskLast_30D()),
			DiskUsage180: periodUtilizationText(util.GetDiskLast_180D()),
		})
	}
	return out
}

type mwRec struct {
	Name, MwType, IP, CPU, Mem, CloudLabel, Region, SysNodeName, DiskStr string
	CpuPeak30, CpuAvg30, MemPeak30, MenAvg30                             string
}

func buildMiddlewares(rdsResp *pbrds.ListResp, redisResp *pbredis.ListResp, kafkaResp *pbkafka.ListResp) []mwRec {
	var out []mwRec
	if rdsResp != nil {
		for _, r := range rdsResp.Rdses {
			ip := firstIP(r.GetPrivateIps(), r.GetPublicIps())
			util := r.GetUtilizationAudit()
			out = append(out, mwRec{
				Name:        r.GetInstanceName(),
				MwType:      "RDS_INS",
				IP:          ip,
				CPU:         itoa32(r.GetCpu()),
				Mem:         memGBStringFromMB(r.GetMemoryMb()),
				CloudLabel:  cloudTypeLabel(r.GetProvider()),
				Region:      r.GetRegionName(),
				SysNodeName: effectiveSysNodeName(r.GetProvider(), r.GetRegionName(), r.GetNodeTagValue()),
				DiskStr:     "", // 列表 API 无独立磁盘字段
				CpuPeak30:   utilWindowPeakText(util.GetCpuLast_30D()),
				CpuAvg30:    utilWindowAvgText(util.GetCpuLast_30D()),
				MemPeak30:   utilWindowPeakText(util.GetMemLast_30D()),
				MenAvg30:    utilWindowAvgText(util.GetMemLast_30D()),
			})
		}
	}
	if redisResp != nil {
		for _, r := range redisResp.Redises {
			ip := firstIP(r.GetPrivateIps(), r.GetPublicIps())
			memUtil := r.GetMemoryUtilizationAudit()
			out = append(out, mwRec{
				Name:        r.GetInstanceName(),
				MwType:      "DCS_REDIS",
				IP:          ip,
				CPU:         itoa32(r.GetCpu()),
				Mem:         memGBStringFromMB(r.GetSize()), // 华为 DCS size 为 MB，与 cmdb 展示一致按 GB 字符串
				CloudLabel:  cloudTypeLabel(r.GetProvider()),
				Region:      r.GetRegionName(),
				SysNodeName: effectiveSysNodeName(r.GetProvider(), r.GetRegionName(), r.GetNodeTagValue()),
				DiskStr:     itoa32(r.GetSize()) + "MB", // 容量规格（与内存 GB 独立对比）
				CpuPeak30:   "",
				CpuAvg30:    "",
				MemPeak30:   utilWindowPeakText(memUtil.GetMemLast_30D()),
				MenAvg30:    utilWindowAvgText(memUtil.GetMemLast_30D()),
			})
		}
	}
	if kafkaResp != nil {
		for _, k := range kafkaResp.Kafkas {
			out = append(out, mwRec{
				Name:        k.GetInstanceName(),
				MwType:      "DMS_ROCKETMQ",
				IP:          strings.TrimSpace(k.GetEndPoint()),
				CPU:         "",
				Mem:         "",
				CloudLabel:  cloudTypeLabel(k.GetProvider()),
				Region:      k.GetRegionName(),
				SysNodeName: effectiveSysNodeName(k.GetProvider(), k.GetRegionName(), k.GetNodeTagValue()),
				DiskStr:     itoa32(k.GetDistSize()) + "GB",
				CpuPeak30:   "",
				CpuAvg30:    "",
				MemPeak30:   "",
				MenAvg30:    "",
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

func (s *Syncer) addCMDBSystemNodes(systemID, systemName string, nodes map[string]struct{}) {
	for node := range nodes {
		exists, err := s.Client.GetCIID(map[string]any{
			"q": fmt.Sprintf("_type:system_node,sys_node_name:%s,system_id:%s", node, systemID),
		})
		if err != nil {
			glog.Errorf("cmdb get system_node: %v", err)
			continue
		}
		if exists != "" {
			glog.Infof("cmdb: system node %q exists id=%s", node, exists)
			continue
		}
		root, err := s.Client.GetCIID(map[string]any{
			"q": fmt.Sprintf("_type:system,system_id:%s", systemID),
		})
		if err != nil || root == "" {
			glog.Errorf("cmdb: system %s for node %s: root ci err=%v", systemID, node, err)
			continue
		}
		rel, err := s.Client.GetSystemLevelRelations(map[string]any{
			"root_id":  root,
			"level":    "1,2,3",
			"reverse":  1,
		})
		if err != nil {
			glog.Errorf("cmdb relations: %v", err)
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
			glog.Errorf("cmdb: system %s node %s: no relations from root %s", systemID, node, root)
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
		}
		d, err := s.Client.AddCI(payload)
		if err != nil {
			glog.Errorf("cmdb add system_node: %v", err)
			continue
		}
		glog.Infof("cmdb add system_node ok %q: %+v", node, d)
	}
}

func (s *Syncer) addCMDBK8sClusters(systemID string, k8s []k8sCluster, clusterToECS map[string][]string) {
	for _, c := range k8s {
		if c.Name == "" {
			continue
		}
		sysNode := c.SysNodeName
		if sysNode == "" {
			continue
		}
		exists, err := s.Client.GetCIID(map[string]any{
			"q": fmt.Sprintf("_type:k8s_cluster,k8s_cluster_name:%s,system_id:%s,sys_node_name:%s", c.Name, systemID, sysNode),
		})
		if err != nil {
			glog.Errorf("cmdb get k8s: %v", err)
			continue
		}
		if exists != "" {
			glog.Infof("cmdb: k8s %q exists id=%s", c.Name, exists)
			continue
		}
		ips := strings.Join(clusterToECS[c.Name], ",")
		ct, locn := cmdbCloudLocationFromSysNodeName(sysNode, c.CloudLabel, c.Region)
		payload := map[string]any{
			"uuid":              uuid.NewString(),
			"k8s_uuid":          uuid.NewString(),
			"ci_type":           "k8s_cluster",
			"system_id":         systemID,
			"sys_node_name":     sysNode,
			"k8s_cluster_name":  c.Name,
			"host_ip_new":       ips,
			"k8s_version":       c.Version,
			"cloud_type":        ct,
			"location":          locn,
		}
		d, err := s.Client.AddCI(payload)
		if err != nil {
			glog.Errorf("cmdb add k8s: %v", err)
			continue
		}
		glog.Infof("cmdb add k8s ok %q: %+v", c.Name, d)
	}
}

func (s *Syncer) addCMDBHosts(systemID string, hosts []hostRec) {
	for _, h := range hosts {
		if h.IP == "" {
			continue
		}
		sysNode := h.SysNodeName
		if sysNode == "" {
			continue
		}
		q := map[string]any{
			"q": fmt.Sprintf("_type:server,sys_node_name:%s,system_id:%s,private_ip:%s", sysNode, systemID, h.IP),
		}
		exists, err := s.Client.GetCIID(q)
		if err != nil {
			glog.Errorf("cmdb get server: %v", err)
			continue
		}
		if exists != "" {
			row, err := s.Client.GetCIFirst(q)
			if err != nil {
				glog.Errorf("cmdb get server row: %v", err)
				continue
			}
			if row != nil && !serverResourceChanged(row, h) {
				glog.Infof("cmdb: server %q %s cpu/mem/disk 未变 id=%s", h.Name, h.IP, exists)
				continue
			}
			_, err = s.Client.UpdateCI(exists, map[string]any{
				"cpu_count":      int(h.CPU),
				"ram_size":       h.MemGBStr,
				"disk_size":      h.DiskStr,
				"cpu_peak_30":    h.CpuPeak30,
				"cpu_peak_180":   h.CpuPeak180,
				"cpu_avg_30":     h.CpuAvg30,
				"cpu_avg_180":    h.CpuAvg180,
				"mem_peak_30":    h.MemPeak30,
				"men_peak_180":   h.MenPeak180,
				"men_avg_30":     h.MemAvg30,
				"men_avg_180":    h.MenAvg180,
				"disk_usage_30":  h.DiskUsage30,
				"disk_usage_180": h.DiskUsage180,
			})
			if err != nil {
				glog.Errorf("cmdb update server: %v", err)
				continue
			}
			glog.Infof("cmdb update server ok %q %s (cpu/mem/disk)", h.Name, h.IP)
			continue
		}
		ct, locn := cmdbCloudLocationFromSysNodeName(sysNode, h.CloudLabel, h.Region)
		payload := map[string]any{
			"uuid":           uuid.NewString(),
			"ci_type":        "server",
			"system_id":      systemID,
			"sys_node_name":  sysNode,
			"server_name":    h.Name,
			"private_ip":     h.IP,
			"cpu_count":      int(h.CPU),
			"ram_size":       h.MemGBStr,
			"disk_size":      h.DiskStr,
			"os_version":     h.OS,
			"location":       locn,
			"cloud_type":     ct,
			"cpu_peak_30":    h.CpuPeak30,
			"cpu_peak_180":   h.CpuPeak180,
			"cpu_avg_30":     h.CpuAvg30,
			"cpu_avg_180":    h.CpuAvg180,
			"mem_peak_30":    h.MemPeak30,
			"men_peak_180":   h.MenPeak180,
			"men_avg_30":     h.MemAvg30,
			"men_avg_180":    h.MenAvg180,
			"disk_usage_30":  h.DiskUsage30,
			"disk_usage_180": h.DiskUsage180,
		}
		d, err := s.Client.AddCI(payload)
		if err != nil {
			glog.Errorf("cmdb add server: %v", err)
			continue
		}
		glog.Infof("cmdb add server ok %q: %+v", h.Name, d)
	}
}

func (s *Syncer) addCMDBMiddlewares(systemID string, mws []mwRec) {
	for _, m := range mws {
		if m.Name == "" {
			continue
		}
		sysNode := m.SysNodeName
		if sysNode == "" {
			continue
		}
		mq := map[string]any{
			"q": fmt.Sprintf("_type:middle_software,sys_node_name:%s,system_id:%s,resource_name:%s", sysNode, systemID, m.Name),
		}
		exists, err := s.Client.GetCIID(mq)
		if err != nil {
			glog.Errorf("cmdb get middle_software: %v", err)
			continue
		}
		if exists != "" {
			row, err := s.Client.GetCIFirst(mq)
			if err != nil {
				glog.Errorf("cmdb get middle_software row: %v", err)
				continue
			}
			if row != nil && !middlewareResourceChanged(row, m) {
				glog.Infof("cmdb: middleware %q cpu/mem/disk 未变 id=%s", m.Name, exists)
				continue
			}
			_, err = s.Client.UpdateCI(exists, map[string]any{
				"cpu_count":   m.CPU,
				"ram_size":    m.Mem,
				"disk_size":   m.DiskStr,
				"cpu_peak_30": m.CpuPeak30,
				"cpu_avg_30":  m.CpuAvg30,
				"mem_peak_30": m.MemPeak30,
				"men_avg_30":  m.MenAvg30,
			})
			if err != nil {
				glog.Errorf("cmdb update middle_software: %v", err)
				continue
			}
			glog.Infof("cmdb update middleware ok %q (cpu/mem/disk)", m.Name)
			continue
		}
		// 与 cmdb_api.py 中 add_cmdb_middlewares 字段一致（含 location / cloud_type 与 Python 相同赋值方式）
		payload := map[string]any{
			"uuid":          uuid.NewString(),
			"ci_type":       "middle_software",
			"system_id":     systemID,
			"sys_node_name": sysNode,
			"resource_name": m.Name,
			"resource_type": m.MwType,
			"location":      m.CloudLabel,
			"cloud_type":    m.Region,
			"private_ip":    m.IP,
			"cpu_count":     m.CPU,
			"ram_size":      m.Mem,
			"disk_size":     m.DiskStr,
			"cpu_peak_30":   m.CpuPeak30,
			"cpu_avg_30":    m.CpuAvg30,
			"mem_peak_30":   m.MemPeak30,
			"men_avg_30":    m.MenAvg30,
		}
		d, err := s.Client.AddCI(payload)
		if err != nil {
			glog.Errorf("cmdb add middle_software: %v", err)
			continue
		}
		glog.Infof("cmdb add middleware ok %q: %+v", m.Name, d)
	}
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
	return percentText(w.GetPeakPercent())
}

func utilWindowAvgText(w *pbutilization.UtilizationWindow) string {
	if w == nil || !w.GetAvailable() {
		return ""
	}
	return percentText(w.GetAvgPercent())
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

// StartDailyAt 在每天指定本地时刻执行一次 Run（默认 2:00，与需求「每天 2 点」一致）。
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
func CMDBConfigFromEnv() (base, key, secret string, ok bool) {
	base = strings.TrimSpace(os.Getenv("CLOUD_FITTER_CMDB_BASE_URL"))
	key = strings.TrimSpace(os.Getenv("CLOUD_FITTER_CMDB_KEY"))
	secret = strings.TrimSpace(os.Getenv("CLOUD_FITTER_CMDB_SECRET"))
	return base, key, secret, base != "" && key != "" && secret != ""
}
