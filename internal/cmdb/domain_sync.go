package cmdb

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
	"github.com/google/uuid"

	"github.com/cloud-fitter/cloud-fitter/internal/server/eip"
	"github.com/cloud-fitter/cloud-fitter/internal/wafbind"
)

const cmdbDomainCIType = "domain"

// listCMDBDomainTextsForNode 查询 CMDB 中某系统+节点下已有域名 CI（domain_text -> _id）。
func (s *Syncer) listCMDBDomainTextsForNode(systemID, sysNodeName string) (map[string]string, error) {
	out := make(map[string]string)
	if s == nil || s.Client == nil {
		return out, fmt.Errorf("cmdb syncer or client is nil")
	}
	systemID = strings.TrimSpace(systemID)
	sysNodeName = strings.TrimSpace(sysNodeName)
	if systemID == "" || sysNodeName == "" {
		return out, nil
	}
	page := 1
	for {
		data, err := s.Client.GetCI(map[string]any{
			"q":    fmt.Sprintf("_type:%s,system_id:%s,sys_node_name:%s", cmdbDomainCIType, systemID, sysNodeName),
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
			text := strings.TrimSpace(fmt.Sprint(row["domain_text"]))
			if text == "" {
				continue
			}
			ciID := strings.TrimSpace(fmt.Sprint(row["_id"]))
			if ciID == "" {
				continue
			}
			out[text] = ciID
		}
		page++
	}
	return out, nil
}

// domainSyncPlan 计算相对 CMDB 已有 domain_text 需新增与需删除的域名（增量，已存在的不重复添加）。
func domainSyncPlan(existingByText map[string]string, wantDomains []string) (toAdd, toDelete []string) {
	want := domainNamesForCMDB(wantDomains)
	wantSet := make(map[string]struct{}, len(want))
	for _, d := range want {
		wantSet[d] = struct{}{}
		if _, ok := existingByText[d]; !ok {
			toAdd = append(toAdd, d)
		}
	}
	for text := range existingByText {
		if _, ok := wantSet[text]; !ok {
			toDelete = append(toDelete, text)
		}
	}
	return toAdd, toDelete
}

// reconcileNodeDomainCIs 按华为云 WAF 匹配结果同步节点域名 CI：system_id+sys_node_name 定位节点，domain_text 增量增删。
func (s *Syncer) reconcileNodeDomainCIs(systemID, sysNodeName string, wantDomains []string) componentSyncStats {
	st := componentSyncStats{}
	systemID = strings.TrimSpace(systemID)
	sysNodeName = strings.TrimSpace(sysNodeName)
	if systemID == "" || sysNodeName == "" {
		return st
	}
	nodeCIID, err := s.Client.GetCIID(map[string]any{
		"q": fmt.Sprintf("_type:system_node,sys_node_name:%s,system_id:%s", sysNodeName, systemID),
	})
	if err != nil {
		glog.Errorf("cmdb sync domain(node get): system_id=%s node=%q err=%v", systemID, sysNodeName, err)
		st.Errors++
		return st
	}
	if nodeCIID == "" {
		glog.Warningf("cmdb sync domain(skip missing node): system_id=%s node=%q", systemID, sysNodeName)
		st.Skipped++
		return st
	}

	existing, err := s.listCMDBDomainTextsForNode(systemID, sysNodeName)
	if err != nil {
		glog.Errorf("cmdb sync domain(list): system_id=%s node=%q err=%v", systemID, sysNodeName, err)
		st.Errors++
		return st
	}
	want := domainNamesForCMDB(wantDomains)
	// 云侧未匹配到域名时，保留 CMDB 已有 domain CI，不因空匹配删除。
	if len(want) == 0 && len(existing) > 0 {
		glog.Infof("cmdb sync domain(skip preserve on empty cloud): system_id=%s node=%q existing=%d", systemID, sysNodeName, len(existing))
		st.Skipped++
		return st
	}
	toAdd, toDelete := domainSyncPlan(existing, wantDomains)
	if len(toAdd) == 0 && len(toDelete) == 0 {
		glog.V(4).Infof("cmdb sync domain(skip unchanged): system_id=%s node=%q count=%d", systemID, sysNodeName, len(want))
		st.Skipped++
		return st
	}

	for _, text := range toAdd {
		payload := map[string]any{
			"uuid":          uuid.NewString(),
			"ci_type":       cmdbDomainCIType,
			"domain_text":   text,
			"system_id":     systemID,
			"sys_node_name": sysNodeName,
		}
		if _, err := s.Client.AddCI(payload); err != nil {
			glog.Errorf("cmdb sync domain(add): system_id=%s node=%q domain=%q err=%v", systemID, sysNodeName, text, err)
			st.Errors++
			continue
		}
		glog.Infof("cmdb sync domain(add ok): system_id=%s node=%q domain=%q", systemID, sysNodeName, text)
		st.Added++
	}

	for _, text := range toDelete {
		ciID := existing[text]
		if ciID == "" {
			continue
		}
		if _, err := s.Client.DeleteCI(ciID); err != nil {
			glog.Errorf("cmdb sync domain(delete): system_id=%s node=%q domain=%q id=%s err=%v", systemID, sysNodeName, text, ciID, err)
			st.Errors++
			continue
		}
		glog.Infof("cmdb sync domain(delete ok): system_id=%s node=%q domain=%q id=%s", systemID, sysNodeName, text, ciID)
		st.Deleted++
	}
	return st
}

// syncNodeDomainCIsFromBind 对本系统 EIP 推导出的每个节点，用 domain CI 同步 WAF 匹配域名（不再写 system_node.domain_name）。
func (s *Syncer) syncNodeDomainCIsFromBind(systemID string, eips []*eip.Instance, bind wafbind.Result) componentSyncStats {
	_, nodeDomains := wafDomainMapsFromBind(bind)
	var st componentSyncStats
	seenNode := make(map[string]struct{})
	for _, e := range eips {
		if e == nil {
			continue
		}
		nodeKey := wafbind.SysNodeKeyFromEIP(e)
		if nodeKey == "" {
			continue
		}
		if _, ok := seenNode[nodeKey]; ok {
			continue
		}
		seenNode[nodeKey] = struct{}{}
		domains := nodeDomains[nodeKey]
		if domains == nil {
			domains = []string{}
		}
		st = addComponentStats(st, s.reconcileNodeDomainCIs(systemID, nodeKey, domains))
	}
	return st
}
