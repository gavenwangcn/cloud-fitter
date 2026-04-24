package configstore

import (
	"os"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
)

// MySQLDSNFromEnv 从环境变量组装 DSN（密码可含特殊字符）。
// 需要：CLOUD_FITTER_MYSQL_HOST、CLOUD_FITTER_MYSQL_USER、CLOUD_FITTER_MYSQL_DATABASE；
// 可选：CLOUD_FITTER_MYSQL_PASSWORD（默认可为空）、CLOUD_FITTER_MYSQL_PORT（默认 3306）。
func MySQLDSNFromEnv() (string, error) {
	host := strings.TrimSpace(os.Getenv("CLOUD_FITTER_MYSQL_HOST"))
	user := strings.TrimSpace(os.Getenv("CLOUD_FITTER_MYSQL_USER"))
	pass := os.Getenv("CLOUD_FITTER_MYSQL_PASSWORD")
	dbname := strings.TrimSpace(os.Getenv("CLOUD_FITTER_MYSQL_DATABASE"))
	port := strings.TrimSpace(os.Getenv("CLOUD_FITTER_MYSQL_PORT"))
	if port == "" {
		port = "3306"
	}
	if host == "" || user == "" || dbname == "" {
		return "", errors.New("CLOUD_FITTER_MYSQL_HOST、CLOUD_FITTER_MYSQL_USER、CLOUD_FITTER_MYSQL_DATABASE 不能为空")
	}
	loc, err := time.LoadLocation("Local")
	if err != nil {
		loc = time.UTC
	}
	cfg := mysqlDriver.NewConfig()
	cfg.User = user
	cfg.Passwd = pass
	cfg.Net = "tcp"
	cfg.Addr = host + ":" + port
	cfg.DBName = dbname
	cfg.ParseTime = true
	cfg.Loc = loc
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	cfg.Params["charset"] = "utf8mb4"
	cfg.Params["collation"] = "utf8mb4_unicode_ci"
	return cfg.FormatDSN(), nil
}

// UseMySQLFromEnv 当 CLOUD_FITTER_DB_DRIVER 为 mysql（不区分大小写）时为 true。
func UseMySQLFromEnv() bool {
	v := strings.TrimSpace(os.Getenv("CLOUD_FITTER_DB_DRIVER"))
	return strings.EqualFold(v, "mysql")
}
