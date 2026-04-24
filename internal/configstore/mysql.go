package configstore

import (
	"database/sql"
	"strings"

	"github.com/pkg/errors"
	_ "github.com/go-sql-driver/mysql"
)

// migrateSQLMySQL 与 migrateSQLSQLite 表结构一致：cloud_configs、systems。
const migrateSQLMySQL = `
CREATE TABLE IF NOT EXISTS cloud_configs (
	id BIGINT NOT NULL AUTO_INCREMENT,
	provider INT NOT NULL,
	name VARCHAR(255) NOT NULL,
	access_id VARCHAR(512) NOT NULL,
	access_secret VARCHAR(2048) NOT NULL,
	PRIMARY KEY (id),
	UNIQUE KEY uk_cloud_configs_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS systems (
	id BIGINT NOT NULL AUTO_INCREMENT,
	name VARCHAR(512) NOT NULL,
	intro TEXT NOT NULL,
	system_id VARCHAR(256) NOT NULL,
	account_ids TEXT NOT NULL,
	online_time VARCHAR(64) NOT NULL DEFAULT '',
	status VARCHAR(64) NOT NULL DEFAULT '',
	PRIMARY KEY (id),
	UNIQUE KEY uk_systems_system_id (system_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
`

// OpenMySQL 使用 MySQL 连接串打开 Store（连接池，适合多并发查询）。
// dsn 示例：user:pass@tcp(127.0.0.1:3306)/dbname?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci&loc=Local
func OpenMySQL(dsn string) (*Store, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, errors.New("mysql dsn 为空")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, errors.WithMessage(err, "sql open mysql")
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	s := &Store{db: db}
	if err := migrateMySQL(s.db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func migrateMySQL(db *sql.DB) error {
	_, err := db.Exec(migrateSQLMySQL)
	if err != nil {
		return errors.WithMessage(err, "migrate mysql")
	}
	return nil
}
