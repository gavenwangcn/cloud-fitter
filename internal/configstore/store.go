package configstore

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/golang/glog"
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
	account_ids TEXT NOT NULL DEFAULT '[]',
	online_time TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT ''
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
	if err != nil {
		return errors.WithMessage(err, "migrate")
	}
	// 兼容历史库：补齐 systems 新增字段。
	for _, stmt := range []string{
		`ALTER TABLE systems ADD COLUMN online_time TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE systems ADD COLUMN status TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := s.db.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return errors.WithMessage(err, "migrate alter systems columns")
		}
	}
	return nil
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
	OnlineTime   string   `json:"onlineTime"`
	Status       string   `json:"status"`
	AccountIDs   []int64  `json:"accountIds"`
	AccountNames []string `json:"accountNames"`
}

// List 返回全部配置（不含 AK/SK）。
func (s *Store) List() ([]Row, error) {
	start := time.Now()
	glog.Infof("configstore.List begin")
	rows, err := s.db.Query(`SELECT id, provider, name FROM cloud_configs ORDER BY id`)
	if err != nil {
		glog.Warningf("configstore.List query failed err=%v", err)
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
	if err := rows.Err(); err != nil {
		glog.Warningf("configstore.List rows iteration failed err=%v", err)
		return nil, err
	}
	glog.Infof("configstore.List end rows=%d elapsed=%v", len(out), time.Since(start))
	return out, nil
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
	start := time.Now()
	glog.Infof("configstore.ListSystems begin")
	// 先查 cloud_configs，避免 SQLite 单连接下 systems rows 打开时再次查询导致阻塞。
	cfgRows, err := s.List()
	if err != nil {
		glog.Warningf("configstore.ListSystems list configs failed err=%v", err)
		return nil, errors.WithMessage(err, "list configs")
	}
	rows, err := s.db.Query(`SELECT id, name, intro, system_id, online_time, status, account_ids FROM systems ORDER BY id`)
	if err != nil {
		glog.Warningf("configstore.ListSystems query systems failed err=%v", err)
		return nil, errors.WithMessage(err, "query systems")
	}
	defer rows.Close()
	cfgNameByID := make(map[int64]string, len(cfgRows))
	for _, c := range cfgRows {
		cfgNameByID[c.ID] = c.Name
	}

	var out []SystemRow
	for rows.Next() {
		var r SystemRow
		var accountIDsRaw string
		if err := rows.Scan(&r.ID, &r.Name, &r.Intro, &r.SystemID, &r.OnlineTime, &r.Status, &accountIDsRaw); err != nil {
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
	if err := rows.Err(); err != nil {
		glog.Warningf("configstore.ListSystems rows iteration failed err=%v", err)
		return nil, err
	}
	glog.Infof("configstore.ListSystems end systems=%d elapsed=%v", len(out), time.Since(start))
	return out, nil
}

func (s *Store) CreateSystem(name, intro, systemID, onlineTime, status string, accountIDs []int64) error {
	idsJSON, err := json.Marshal(accountIDs)
	if err != nil {
		return errors.WithMessage(err, "marshal account ids")
	}
	_, err = s.db.Exec(
		`INSERT INTO systems (name, intro, system_id, online_time, status, account_ids) VALUES (?, ?, ?, ?, ?, ?)`,
		name, intro, systemID, onlineTime, status, string(idsJSON),
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
