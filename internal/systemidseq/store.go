// Package systemidseq 将系统ID序号持久化在 data 目录下的文本文件中（YH / D 各自递增）。
package systemidseq

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/pkg/errors"
)

// Store 每个前缀单独文件：system_id_seq_YH.txt、system_id_seq_D.txt，内容为十进制整数（上次已分配的序号）。
type Store struct {
	dir string
	mu  sync.Mutex
	// maxSuffixFromDB 可选：返回库里已有 system_id（同前缀）的最大数字，与文件中的序号取 max 后再 +1。
	maxSuffixFromDB func(kind string) (uint64, error)
}

// SetMaxSuffixQuery 设置从数据库合并最大序号的回调（kind 为 YH 或 D）。
func (s *Store) SetMaxSuffixQuery(fn func(kind string) (uint64, error)) {
	s.maxSuffixFromDB = fn
}

// DataDirFromEnvOr 优先环境变量 CLOUD_FITTER_DATA_DIR，否则使用命令行默认值。
func DataDirFromEnvOr(flagDefault string) string {
	if v := strings.TrimSpace(os.Getenv("CLOUD_FITTER_DATA_DIR")); v != "" {
		return v
	}
	return flagDefault
}

// Open 确保目录存在。
func Open(dir string) (*Store, error) {
	d := filepath.Clean(dir)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return nil, errors.Wrap(err, "systemidseq mkdir")
	}
	return &Store{dir: d}, nil
}

// Next 分配下一个系统ID。kind 仅支持 "YH"（生成 YH-000001）或 "D"（生成 D-000001）。
// 序号 = max(文件中已记录的序号, 数据库中同前缀 system_id 的最大数字) + 1。
func (s *Store) Next(kind string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := strings.ToUpper(strings.TrimSpace(kind))
	var fname string
	switch k {
	case "YH":
		fname = "system_id_seq_YH.txt"
	case "D":
		fname = "system_id_seq_D.txt"
	default:
		return "", errors.New("kind must be YH or D")
	}

	fp := filepath.Join(s.dir, fname)
	fileN, err := readUint(fp)
	if err != nil {
		return "", err
	}
	dbMax := uint64(0)
	if s.maxSuffixFromDB != nil {
		m, err := s.maxSuffixFromDB(k)
		if err != nil {
			return "", err
		}
		dbMax = m
	}
	last := fileN
	if dbMax > last {
		last = dbMax
	}
	n := last + 1
	out := fmt.Sprintf("%s-%06d", k, n)
	if err := writeUint(fp, n); err != nil {
		return "", err
	}
	return out, nil
}

func readUint(path string) (uint64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, errors.Wrap(err, "read seq file")
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return 0, nil
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "parse seq file %s", path)
	}
	return n, nil
}

func writeUint(path string, n uint64) error {
	data := []byte(strconv.FormatUint(n, 10) + "\n")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return errors.Wrap(err, "write seq tmp")
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return errors.Wrap(err, "rename seq file")
	}
	return nil
}
