package resourcecache

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/pkg/errors"
)

// ReplaceHuaweiEcsCceMapSnapshot 写入华为 ECS 实例 ID -> CCE 集群 metadata.uid 映射（单行 JSON 对象）。m 可为 nil 或空，表示清空该系统的映射。
func ReplaceHuaweiEcsCceMapSnapshot(ctx context.Context, db *sql.DB, systemID string, m map[string]string) error {
	if db == nil {
		return errors.New("resourcecache ReplaceHuaweiEcsCceMapSnapshot: nil db")
	}
	if m == nil {
		m = map[string]string{}
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return errors.Wrap(err, "marshal ecs_cce map")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "DELETE FROM "+TableEcsCceMap+" WHERE system_id = ?", systemID); err != nil {
		return errors.Wrap(err, "delete cloud_snap_ecs_cce_map")
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	if _, err := tx.ExecContext(ctx, "INSERT INTO "+TableEcsCceMap+" (system_id, map_json, updated_at) VALUES (?,?,?)", systemID, string(raw), now); err != nil {
		return errors.Wrap(err, "insert cloud_snap_ecs_cce_map")
	}
	return tx.Commit()
}

// LoadHuaweiEcsCceMap 读取映射；无行返回 (nil, nil)；JSON 非法返回错误。
func LoadHuaweiEcsCceMap(ctx context.Context, db *sql.DB, systemID string) (map[string]string, error) {
	if db == nil {
		return nil, errors.New("resourcecache LoadHuaweiEcsCceMap: nil db")
	}
	var raw string
	err := db.QueryRowContext(ctx, "SELECT map_json FROM "+TableEcsCceMap+" WHERE system_id = ?", systemID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, errors.Wrap(err, "unmarshal ecs_cce map")
	}
	if out == nil {
		out = map[string]string{}
	}
	return out, nil
}
