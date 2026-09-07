package billingbatchexport

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const defaultSubdir = "batch-export"

// ExportDir 批量导出 xlsx 存放目录。
// 优先 CLOUD_FITTER_BILLING_BATCH_EXPORT_DIR；否则 ${CLOUD_FITTER_DATA_DIR:-data}/batch-export。
func ExportDir() string {
	if v := strings.TrimSpace(os.Getenv("CLOUD_FITTER_BILLING_BATCH_EXPORT_DIR")); v != "" {
		return filepath.Clean(v)
	}
	base := strings.TrimSpace(os.Getenv("CLOUD_FITTER_DATA_DIR"))
	if base == "" {
		base = "data"
	}
	return filepath.Join(base, defaultSubdir)
}

// EnsureDir 创建导出目录（若不存在）。
func EnsureDir() error {
	return os.MkdirAll(ExportDir(), 0o755)
}

// NewFilename 按东八区点击时刻（到分钟）生成 xlsx 文件名；同分钟冲突时追加 -2、-3…
func NewFilename(now time.Time) (string, error) {
	if err := EnsureDir(); err != nil {
		return "", err
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.UTC
	}
	t := now.In(loc)
	base := t.Format("20060102-1504")
	dir := ExportDir()
	for i := 0; i < 100; i++ {
		name := base + ".xlsx"
		if i > 0 {
			name = fmt.Sprintf("%s-%d.xlsx", base, i+1)
		}
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			return name, nil
		}
		if err != nil {
			return "", errors.WithMessage(err, "stat export file")
		}
	}
	return "", errors.Errorf("too many export files for minute prefix %s", base)
}

// ListXLSX 列出目录下已完成导出的 .xlsx（不含 .part），按文件名降序（新文件在前）。
func ListXLSX() ([]string, error) {
	dir := ExportDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.WithMessage(err, "read export dir")
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasSuffix(strings.ToLower(n), ".xlsx") && !strings.HasSuffix(n, ".part") {
			names = append(names, n)
		}
	}
	sort.Slice(names, func(i, j int) bool { return names[i] > names[j] })
	return names, nil
}

// ResolveFile 校验文件名并返回绝对路径（防目录穿越）。
func ResolveFile(filename string) (string, error) {
	name := strings.TrimSpace(filename)
	if name == "" {
		return "", errors.New("filename is required")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return "", errors.New("invalid filename")
	}
	if !strings.HasSuffix(strings.ToLower(name), ".xlsx") {
		return "", errors.New("only .xlsx files are allowed")
	}
	dir := filepath.Clean(ExportDir())
	abs := filepath.Join(dir, name)
	if filepath.Dir(abs) != dir {
		return "", errors.New("invalid filename")
	}
	return abs, nil
}
