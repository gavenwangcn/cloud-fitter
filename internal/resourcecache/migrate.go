package resourcecache

import (
	"database/sql"

	"github.com/pkg/errors"
)

// MigrateMySQL 创建云资源快照表（与 CMDB 定时任务解耦：先定时拉 API 落库，凌晨任务可读库不写云接口）。
const migrateMySQLSnapECS = `
CREATE TABLE IF NOT EXISTS cloud_snap_ecs (
	system_id VARCHAR(256) NOT NULL,
	system_name VARCHAR(512) NOT NULL DEFAULT '',
	resource_key VARCHAR(128) NOT NULL,
	sys_node_key VARCHAR(512) NOT NULL DEFAULT '',
	payload_json MEDIUMTEXT NOT NULL,
	updated_at DATETIME(3) NOT NULL,
	PRIMARY KEY (system_id, resource_key),
	KEY idx_cloud_snap_ecs_name (system_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const migrateMySQLSnapRDS = `
CREATE TABLE IF NOT EXISTS cloud_snap_rds (
	system_id VARCHAR(256) NOT NULL,
	system_name VARCHAR(512) NOT NULL DEFAULT '',
	resource_key VARCHAR(128) NOT NULL,
	sys_node_key VARCHAR(512) NOT NULL DEFAULT '',
	payload_json MEDIUMTEXT NOT NULL,
	updated_at DATETIME(3) NOT NULL,
	PRIMARY KEY (system_id, resource_key),
	KEY idx_cloud_snap_rds_name (system_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const migrateMySQLSnapDCS = `
CREATE TABLE IF NOT EXISTS cloud_snap_dcs (
	system_id VARCHAR(256) NOT NULL,
	system_name VARCHAR(512) NOT NULL DEFAULT '',
	resource_key VARCHAR(128) NOT NULL,
	sys_node_key VARCHAR(512) NOT NULL DEFAULT '',
	payload_json MEDIUMTEXT NOT NULL,
	updated_at DATETIME(3) NOT NULL,
	PRIMARY KEY (system_id, resource_key),
	KEY idx_cloud_snap_dcs_name (system_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const migrateMySQLSnapDMS = `
CREATE TABLE IF NOT EXISTS cloud_snap_dms_kafka (
	system_id VARCHAR(256) NOT NULL,
	system_name VARCHAR(512) NOT NULL DEFAULT '',
	resource_key VARCHAR(128) NOT NULL,
	sys_node_key VARCHAR(512) NOT NULL DEFAULT '',
	payload_json MEDIUMTEXT NOT NULL,
	updated_at DATETIME(3) NOT NULL,
	PRIMARY KEY (system_id, resource_key),
	KEY idx_cloud_snap_dms_name (system_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const migrateMySQLSnapCCE = `
CREATE TABLE IF NOT EXISTS cloud_snap_cce (
	system_id VARCHAR(256) NOT NULL,
	system_name VARCHAR(512) NOT NULL DEFAULT '',
	resource_key VARCHAR(128) NOT NULL,
	sys_node_key VARCHAR(512) NOT NULL DEFAULT '',
	payload_json MEDIUMTEXT NOT NULL,
	updated_at DATETIME(3) NOT NULL,
	PRIMARY KEY (system_id, resource_key),
	KEY idx_cloud_snap_cce_name (system_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const migrateMySQLSnapEIP = `
CREATE TABLE IF NOT EXISTS cloud_snap_eip (
	system_id VARCHAR(256) NOT NULL,
	system_name VARCHAR(512) NOT NULL DEFAULT '',
	resource_key VARCHAR(128) NOT NULL,
	sys_node_key VARCHAR(512) NOT NULL DEFAULT '',
	payload_json MEDIUMTEXT NOT NULL,
	updated_at DATETIME(3) NOT NULL,
	PRIMARY KEY (system_id, resource_key),
	KEY idx_cloud_snap_eip_name (system_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const migrateMySQLSnapELB = `
CREATE TABLE IF NOT EXISTS cloud_snap_elb (
	system_id VARCHAR(256) NOT NULL,
	system_name VARCHAR(512) NOT NULL DEFAULT '',
	resource_key VARCHAR(128) NOT NULL,
	sys_node_key VARCHAR(512) NOT NULL DEFAULT '',
	payload_json MEDIUMTEXT NOT NULL,
	updated_at DATETIME(3) NOT NULL,
	PRIMARY KEY (system_id, resource_key),
	KEY idx_cloud_snap_elb_name (system_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const migrateMySQLSnapBilling = `
CREATE TABLE IF NOT EXISTS cloud_snap_billing (
	system_id VARCHAR(256) NOT NULL,
	system_name VARCHAR(512) NOT NULL DEFAULT '',
	billing_month VARCHAR(7) NOT NULL,
	provider INT NOT NULL,
	account_name VARCHAR(255) NOT NULL,
	payload_json MEDIUMTEXT NOT NULL,
	updated_at DATETIME(3) NOT NULL,
	PRIMARY KEY (system_id, billing_month, provider, account_name),
	KEY idx_cloud_snap_billing_month (billing_month)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const migrateMySQLSnapEcsCceMap = `
CREATE TABLE IF NOT EXISTS cloud_snap_ecs_cce_map (
	system_id VARCHAR(256) NOT NULL,
	map_json MEDIUMTEXT NOT NULL,
	updated_at DATETIME(3) NOT NULL,
	PRIMARY KEY (system_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const migrateMySQLSnapWAF = `
CREATE TABLE IF NOT EXISTS cloud_snap_waf (
	system_id VARCHAR(256) NOT NULL,
	system_name VARCHAR(512) NOT NULL DEFAULT '',
	resource_key VARCHAR(128) NOT NULL,
	sys_node_key VARCHAR(512) NOT NULL DEFAULT '',
	payload_json MEDIUMTEXT NOT NULL,
	updated_at DATETIME(3) NOT NULL,
	PRIMARY KEY (system_id, resource_key),
	KEY idx_cloud_snap_waf_name (system_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const migrateMySQLSnapCertificate = `
CREATE TABLE IF NOT EXISTS cloud_snap_certificate (
	system_id VARCHAR(256) NOT NULL,
	system_name VARCHAR(512) NOT NULL DEFAULT '',
	resource_key VARCHAR(128) NOT NULL,
	sys_node_key VARCHAR(512) NOT NULL DEFAULT '',
	payload_json MEDIUMTEXT NOT NULL,
	updated_at DATETIME(3) NOT NULL,
	PRIMARY KEY (system_id, resource_key),
	KEY idx_cloud_snap_certificate_name (system_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const migrateMySQLSnapWAFEIPDomain = `
CREATE TABLE IF NOT EXISTS cloud_snap_waf_eip_domain (
	system_id VARCHAR(256) NOT NULL,
	system_name VARCHAR(512) NOT NULL DEFAULT '',
	eip_resource_key VARCHAR(128) NOT NULL,
	sys_node_key VARCHAR(512) NOT NULL DEFAULT '',
	domain_names_json MEDIUMTEXT NOT NULL,
	updated_at DATETIME(3) NOT NULL,
	PRIMARY KEY (system_id, eip_resource_key),
	KEY idx_cloud_snap_waf_eip_domain_name (system_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const migrateMySQLSnapWAFNodeDomain = `
CREATE TABLE IF NOT EXISTS cloud_snap_waf_node_domain (
	system_id VARCHAR(256) NOT NULL,
	system_name VARCHAR(512) NOT NULL DEFAULT '',
	sys_node_key VARCHAR(512) NOT NULL,
	domain_names_json MEDIUMTEXT NOT NULL,
	updated_at DATETIME(3) NOT NULL,
	PRIMARY KEY (system_id, sys_node_key),
	KEY idx_cloud_snap_waf_node_domain_name (system_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

// MigrateMySQL 在已有业务库上执行建表（幂等）。
func MigrateMySQL(db *sql.DB) error {
	stmts := []string{
		migrateMySQLSnapECS, migrateMySQLSnapRDS, migrateMySQLSnapDCS,
		migrateMySQLSnapDMS, migrateMySQLSnapCCE, migrateMySQLSnapEIP, migrateMySQLSnapELB,
		migrateMySQLSnapBilling, migrateMySQLSnapEcsCceMap,
		migrateMySQLSnapWAF, migrateMySQLSnapCertificate,
		migrateMySQLSnapWAFEIPDomain, migrateMySQLSnapWAFNodeDomain,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			return errors.Wrap(err, "resourcecache migrate mysql")
		}
	}
	return nil
}
