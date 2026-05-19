package resourcecache

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/internal/server/cert"
	"github.com/cloud-fitter/cloud-fitter/internal/server/waf"
	"github.com/cloud-fitter/cloud-fitter/internal/wafbind"
)

const (
	TableWAF           = "cloud_snap_waf"
	TableCertificate   = "cloud_snap_certificate"
	TableWAF_EIPDomain = "cloud_snap_waf_eip_domain"
	TableWAF_NodeDomain = "cloud_snap_waf_node_domain"
)

// ReplaceWafSnapshot 全量覆盖 WAF 防护域名快照（rows 应仅含 CLOUD_FITTER_HUAWEI_WAF_CMDB_ACCOUNT_NAMES 指定账号数据）。
func ReplaceWafSnapshot(ctx context.Context, db *sql.DB, systemID, systemName string, rows []*waf.Instance) error {
	snap := make([]SnapshotRow, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		key := ResourceKeyECS(strings.TrimSpace(row.ID))
		if key == "" {
			key = ResourceKeyECS(strings.TrimSpace(row.HostID))
		}
		if key == "" {
			continue
		}
		b, err := json.Marshal(row)
		if err != nil {
			continue
		}
		snap = append(snap, SnapshotRow{
			ResourceKey: key,
			SysNodeKey:  strings.TrimSpace(row.RegionName),
			PayloadJSON: string(b),
		})
	}
	return ReplaceSnapshotTable(ctx, db, TableWAF, systemID, systemName, snap)
}

// ReplaceCertificateSnapshot 全量覆盖 SCM 证书快照（rows 应仅含 WAF 环境变量指定账号数据）。
func ReplaceCertificateSnapshot(ctx context.Context, db *sql.DB, systemID, systemName string, rows []*cert.Instance) error {
	seen := make(map[string]struct{})
	snap := make([]SnapshotRow, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		key := ResourceKeyECS(strings.TrimSpace(row.ID))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		b, err := json.Marshal(row)
		if err != nil {
			continue
		}
		snap = append(snap, SnapshotRow{
			ResourceKey: key,
			SysNodeKey:  strings.TrimSpace(row.AccountName),
			PayloadJSON: string(b),
		})
	}
	return ReplaceSnapshotTable(ctx, db, TableCertificate, systemID, systemName, snap)
}

// ReplaceWafDomainSnapshots 写入 WAF 匹配后的 EIP/节点域名绑定镜像。
func ReplaceWafDomainSnapshots(ctx context.Context, db *sql.DB, systemID, systemName string, bind wafbind.Result) error {
	if db == nil {
		return errors.New("resourcecache: nil db")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Truncate(time.Millisecond)

	if _, err := tx.ExecContext(ctx, "DELETE FROM "+TableWAF_EIPDomain+" WHERE system_id = ?", systemID); err != nil {
		return errors.Wrap(err, "delete waf eip domain snap")
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM "+TableWAF_NodeDomain+" WHERE system_id = ?", systemID); err != nil {
		return errors.Wrap(err, "delete waf node domain snap")
	}

	if len(bind.EIPDomains) > 0 {
		const insEIP = `INSERT INTO ` + TableWAF_EIPDomain + ` (system_id, system_name, eip_resource_key, sys_node_key, domain_names_json, updated_at) VALUES (?,?,?,?,?,?)`
		for _, r := range bind.EIPDomains {
			dj, _ := json.Marshal(r.Domains)
			if _, err := tx.ExecContext(ctx, insEIP, systemID, systemName, r.EIPResourceKey, r.SysNodeKey, string(dj), now); err != nil {
				return errors.Wrap(err, "insert waf eip domain snap")
			}
		}
	}
	if len(bind.NodeDomains) > 0 {
		const insNode = `INSERT INTO ` + TableWAF_NodeDomain + ` (system_id, system_name, sys_node_key, domain_names_json, updated_at) VALUES (?,?,?,?,?)`
		for _, r := range bind.NodeDomains {
			dj, _ := json.Marshal(r.Domains)
			if _, err := tx.ExecContext(ctx, insNode, systemID, systemName, r.SysNodeKey, string(dj), now); err != nil {
				return errors.Wrap(err, "insert waf node domain snap")
			}
		}
	}
	return tx.Commit()
}

// LoadWAF 从快照读取 WAF 列表；wafAccountNames 非空时仅保留这些账号（与写入/直连 API 一致）。
func LoadWAF(ctx context.Context, db *sql.DB, systemID string, wafAccountNames []string) ([]*waf.Instance, error) {
	raw, err := loadWAFUnfiltered(ctx, db, systemID)
	if err != nil {
		return nil, err
	}
	filtered := wafbind.FilterWAFRows(raw, wafAccountNames)
	wafbind.LogWAFSnapshotFilter(systemID, wafAccountNames, len(raw), len(filtered))
	return filtered, nil
}

// LoadCertificates 从快照读取证书列表；wafAccountNames 非空时仅保留这些账号。
func LoadCertificates(ctx context.Context, db *sql.DB, systemID string, wafAccountNames []string) ([]*cert.Instance, error) {
	raw, err := loadCertificatesUnfiltered(ctx, db, systemID)
	if err != nil {
		return nil, err
	}
	filtered := wafbind.FilterCertRows(raw, wafAccountNames)
	wafbind.LogCertSnapshotFilter(systemID, wafAccountNames, len(raw), len(filtered))
	return filtered, nil
}

func loadWAFUnfiltered(ctx context.Context, db *sql.DB, systemID string) ([]*waf.Instance, error) {
	var out []*waf.Instance
	rows, err := db.QueryContext(ctx, "SELECT payload_json FROM "+TableWAF+" WHERE system_id = ? ORDER BY resource_key", systemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var m waf.Instance
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			return nil, errors.Wrap(err, "unmarshal waf snapshot")
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

func loadCertificatesUnfiltered(ctx context.Context, db *sql.DB, systemID string) ([]*cert.Instance, error) {
	var out []*cert.Instance
	rows, err := db.QueryContext(ctx, "SELECT payload_json FROM "+TableCertificate+" WHERE system_id = ? ORDER BY resource_key", systemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var m cert.Instance
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			return nil, errors.Wrap(err, "unmarshal certificate snapshot")
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

// LoadWafDomainBindings 读取 EIP/节点域名绑定快照。
func LoadWafDomainBindings(ctx context.Context, db *sql.DB, systemID string) (wafbind.Result, error) {
	var res wafbind.Result
	eipRows, err := db.QueryContext(ctx,
		"SELECT eip_resource_key, sys_node_key, domain_names_json FROM "+TableWAF_EIPDomain+" WHERE system_id = ? ORDER BY eip_resource_key", systemID)
	if err != nil {
		return res, err
	}
	defer eipRows.Close()
	for eipRows.Next() {
		var key, node, dj string
		if err := eipRows.Scan(&key, &node, &dj); err != nil {
			return res, err
		}
		var domains []string
		_ = json.Unmarshal([]byte(dj), &domains)
		res.EIPDomains = append(res.EIPDomains, wafbind.EIPDomainRow{
			EIPResourceKey: key,
			SysNodeKey:     node,
			Domains:        domains,
		})
	}
	if err := eipRows.Err(); err != nil {
		return res, err
	}

	nodeRows, err := db.QueryContext(ctx,
		"SELECT sys_node_key, domain_names_json FROM "+TableWAF_NodeDomain+" WHERE system_id = ? ORDER BY sys_node_key", systemID)
	if err != nil {
		return res, err
	}
	defer nodeRows.Close()
	for nodeRows.Next() {
		var node, dj string
		if err := nodeRows.Scan(&node, &dj); err != nil {
			return res, err
		}
		var domains []string
		_ = json.Unmarshal([]byte(dj), &domains)
		res.NodeDomains = append(res.NodeDomains, wafbind.NodeDomainRow{
			SysNodeKey: node,
			Domains:    domains,
		})
	}
	return res, nodeRows.Err()
}

// HasWafCertSnapshot 是否已有 WAF/证书镜像（用于 CMDB 快照模式判断是否读库）。
func HasWafCertSnapshot(ctx context.Context, db *sql.DB, systemID string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, "SELECT COUNT(1) FROM "+TableWAF+" WHERE system_id = ?", systemID).Scan(&n)
	if err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	err = db.QueryRowContext(ctx, "SELECT COUNT(1) FROM "+TableCertificate+" WHERE system_id = ?", systemID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
