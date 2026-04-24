package configstore

import (
	"database/sql"
	"encoding/json"

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

CREATE TABLE IF NOT EXISTS systems (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	intro TEXT NOT NULL,
	system_id TEXT NOT NULL UNIQUE,
	account_ids TEXT NOT NULL DEFAULT '[]'
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

type SystemRow struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Intro        string   `json:"intro"`
	SystemID     string   `json:"systemId"`
	AccountIDs   []int64  `json:"accountIds"`
	AccountNames []string `json:"accountNames"`
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

func (s *Store) ListSystems() ([]SystemRow, error) {
	rows, err := s.db.Query(`SELECT id, name, intro, system_id, account_ids FROM systems ORDER BY id`)
	if err != nil {
		return nil, errors.WithMessage(err, "query systems")
	}
	defer rows.Close()

	cfgRows, err := s.List()
	if err != nil {
		return nil, errors.WithMessage(err, "list configs")
	}
	cfgNameByID := make(map[int64]string, len(cfgRows))
	for _, c := range cfgRows {
		cfgNameByID[c.ID] = c.Name
	}

	var out []SystemRow
	for rows.Next() {
		var r SystemRow
		var accountIDsRaw string
		if err := rows.Scan(&r.ID, &r.Name, &r.Intro, &r.SystemID, &accountIDsRaw); err != nil {
			return nil, errors.WithMessage(err, "scan systems")
		}
		if accountIDsRaw != "" {
			if err := json.Unmarshal([]byte(accountIDsRaw), &r.AccountIDs); err != nil {
				return nil, errors.WithMessage(err, "decode systems.account_ids")
			}
		}
		for _, id := range r.AccountIDs {
			if n, ok := cfgNameByID[id]; ok {
				r.AccountNames = append(r.AccountNames, n)
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateSystem(name, intro, systemID string, accountIDs []int64) error {
	idsJSON, err := json.Marshal(accountIDs)
	if err != nil {
		return errors.WithMessage(err, "marshal account ids")
	}
	_, err = s.db.Exec(
		`INSERT INTO systems (name, intro, system_id, account_ids) VALUES (?, ?, ?, ?)`,
		name, intro, systemID, string(idsJSON),
	)
	return errors.WithMessage(err, "insert system")
}

func (s *Store) HasSystemID(systemID string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM systems WHERE system_id = ?`, systemID).Scan(&n)
	if err != nil {
		return false, errors.WithMessage(err, "count systems by system_id")
	}
	return n > 0, nil
}

// AccountsBySystemName 按系统名称解析其关联账号（provider + name）。
func (s *Store) AccountsBySystemName(systemName string) ([]Row, error) {
	var raw string
	err := s.db.QueryRow(`SELECT account_ids FROM systems WHERE name = ? LIMIT 1`, systemName).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("system not found")
		}
		return nil, errors.WithMessage(err, "query systems by name")
	}

	var ids []int64
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &ids); err != nil {
			return nil, errors.WithMessage(err, "decode systems.account_ids")
		}
	}
	if len(ids) == 0 {
		return []Row{}, nil
	}

	var out []Row
	for _, id := range ids {
		var r Row
		qerr := s.db.QueryRow(`SELECT id, provider, name FROM cloud_configs WHERE id = ?`, id).
			Scan(&r.ID, &r.Provider, &r.Name)
		if qerr != nil {
			if errors.Is(qerr, sql.ErrNoRows) {
				continue
			}
			return nil, errors.WithMessage(qerr, "query cloud_configs by id")
		}
		out = append(out, r)
	}
	return out, nil
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
