package resourcecache

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
)

// 与 MySQL 中 cloud_snap_* 表名一致。
const (
	TableECS  = "cloud_snap_ecs"
	TableRDS  = "cloud_snap_rds"
	TableDCS  = "cloud_snap_dcs"
	TableDMS  = "cloud_snap_dms_kafka"
	TableCCE  = "cloud_snap_cce"
	TableEIP  = "cloud_snap_eip"
	TableELB       = "cloud_snap_elb"
	TableBilling   = "cloud_snap_billing"
	TableEcsCceMap = "cloud_snap_ecs_cce_map"
)

// SnapshotRow 为单条资源快照行（主键由 system_id + resource_key 在表级约束）。
type SnapshotRow struct {
	ResourceKey string
	SysNodeKey  string
	PayloadJSON string
}

// ReplaceSnapshotTable 在事务内删除该系统该表全部行后批量插入（全量覆盖）。table 须为 resourcecache.Table* 常量。
// 若 rows 为空则不修改库（保留上次成功写入的快照）。
func ReplaceSnapshotTable(ctx context.Context, db *sql.DB, table, systemID, systemName string, rows []SnapshotRow) error {
	if db == nil {
		return errors.New("resourcecache: nil db")
	}
	if len(rows) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE system_id = ?", systemID); err != nil {
		return errors.Wrapf(err, "delete from %s", table)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	const insCols = "(system_id, system_name, resource_key, sys_node_key, payload_json, updated_at)"
	q := "INSERT INTO " + table + " " + insCols + " VALUES (?,?,?,?,?,?)"
	for _, r := range rows {
		if _, err := tx.ExecContext(ctx, q, systemID, systemName, r.ResourceKey, r.SysNodeKey, r.PayloadJSON, now); err != nil {
			return errors.Wrapf(err, "insert %s key=%s", table, r.ResourceKey)
		}
	}
	return tx.Commit()
}
