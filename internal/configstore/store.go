package configstore

import (
	"database/sql"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/pkg/errors"
	_ "modernc.org/sqlite"
)

const migrateSQL = `
CREATE TABLE IF NOT EXISTS cloud_configs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	provider INTEGER NOT NULL,
	name TEXT NOT NULL UNIQUE,
	access_id TEXT NOT NULL,
	access_secret TEXT NOT NULL
);
`

// Store 将云账号配置持久化在 SQLite 中。
type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, errors.WithMessage(err, "sql open")
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.Migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Migrate() error {
	_, err := s.db.Exec(migrateSQL)
	return errors.WithMessage(err, "migrate")
}

// Row 对外展示（不含密钥）。
type Row struct {
	ID       int64 `json:"id"`
	Provider int32 `json:"provider"`
	Name     string `json:"name"`
}

// List 返回全部配置（不含 AK/SK）。
func (s *Store) List() ([]Row, error) {
	rows, err := s.db.Query(`SELECT id, provider, name FROM cloud_configs ORDER BY id`)
	if err != nil {
		return nil, errors.WithMessage(err, "query")
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.Provider, &r.Name); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Create 插入一条配置。
func (s *Store) Create(provider int32, name, accessID, accessSecret string) error {
	_, err := s.db.Exec(
		`INSERT INTO cloud_configs (provider, name, access_id, access_secret) VALUES (?, ?, ?, ?)`,
		provider, name, accessID, accessSecret,
	)
	return errors.WithMessage(err, "insert")
}

// ToCloudConfigs 读取全部行并转为租户加载结构。
func (s *Store) ToCloudConfigs() (*pbtenant.CloudConfigs, error) {
	rows, err := s.db.Query(`SELECT provider, name, access_id, access_secret FROM cloud_configs ORDER BY id`)
	if err != nil {
		return nil, errors.WithMessage(err, "query")
	}
	defer rows.Close()
	cfg := &pbtenant.CloudConfigs{}
	for rows.Next() {
		var provider int32
		var name, ak, sk string
		if err := rows.Scan(&provider, &name, &ak, &sk); err != nil {
			return nil, err
		}
		cfg.Configs = append(cfg.Configs, &pbtenant.CloudConfig{
			Provider:     pbtenant.CloudProvider(provider),
			Name:         name,
			AccessId:     ak,
			AccessSecret: sk,
		})
	}
	return cfg, rows.Err()
}

// Count 返回表中行数。
func (s *Store) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM cloud_configs`).Scan(&n)
	return n, errors.WithMessage(err, "count")
}
