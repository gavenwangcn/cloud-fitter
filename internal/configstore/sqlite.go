package configstore

import (
	"database/sql"
	"strings"

	"github.com/pkg/errors"
	_ "modernc.org/sqlite"
)

const migrateSQLSQLite = `
CREATE TABLE IF NOT EXISTS cloud_configs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	provider INTEGER NOT NULL,
	name TEXT NOT NULL UNIQUE,
	access_id TEXT NOT NULL,
	access_secret TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS systems (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	intro TEXT NOT NULL,
	system_id TEXT NOT NULL UNIQUE,
	account_ids TEXT NOT NULL DEFAULT '[]',
	online_time TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT ''
);
`

// Open 在 SQLite 文件上打开 Store（单连接，与历史行为一致）。
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, errors.WithMessage(err, "sql open sqlite")
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := migrateSQLite(s.db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func migrateSQLite(db *sql.DB) error {
	_, err := db.Exec(migrateSQLSQLite)
	if err != nil {
		return errors.WithMessage(err, "migrate sqlite")
	}
	for _, stmt := range []string{
		`ALTER TABLE systems ADD COLUMN online_time TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE systems ADD COLUMN status TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := db.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return errors.WithMessage(err, "migrate alter systems columns")
		}
	}
	return nil
}
