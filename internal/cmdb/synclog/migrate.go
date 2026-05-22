package synclog

import (
	"database/sql"

	"github.com/pkg/errors"
)

const migrateMySQLSyncRun = `
CREATE TABLE IF NOT EXISTS cmdb_sync_run (
	id BIGINT NOT NULL AUTO_INCREMENT,
	trigger_type VARCHAR(32) NOT NULL DEFAULT 'scheduled',
	manual_system_id VARCHAR(256) NOT NULL DEFAULT '',
	manual_system_name VARCHAR(512) NOT NULL DEFAULT '',
	started_at DATETIME(3) NOT NULL,
	finished_at DATETIME(3) NULL,
	total_systems INT NOT NULL DEFAULT 0,
	success_systems INT NOT NULL DEFAULT 0,
	failed_systems INT NOT NULL DEFAULT 0,
	skipped_systems INT NOT NULL DEFAULT 0,
	status VARCHAR(32) NOT NULL DEFAULT 'running',
	resource_fail_totals_json MEDIUMTEXT NOT NULL,
	systems_detail_json MEDIUMTEXT NOT NULL,
	error_message TEXT NOT NULL,
	PRIMARY KEY (id),
	KEY idx_cmdb_sync_run_started (started_at),
	KEY idx_cmdb_sync_run_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

// MigrateMySQL 创建 CMDB 同步结果表（幂等）。
func MigrateMySQL(db *sql.DB) error {
	if _, err := db.Exec(migrateMySQLSyncRun); err != nil {
		return errors.Wrap(err, "synclog migrate mysql")
	}
	return nil
}
