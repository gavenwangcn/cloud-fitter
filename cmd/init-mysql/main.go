// init-mysql：根据 .env / 环境变量连接 MySQL，执行与 cloud-fitter 一致的建表（cloud_configs、systems）。
// 用法：在仓库根目录执行 go run ./cmd/init-mysql，或 docker compose -f docker-compose-mysql.yml run --rm init-mysql
package main

import (
	"fmt"
	"log"

	"github.com/cloud-fitter/cloud-fitter/internal/configstore"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load(".env")
	dsn, err := configstore.MySQLDSNFromEnv()
	if err != nil {
		log.Fatalf("mysql dsn: %v", err)
	}
	s, err := configstore.OpenMySQL(dsn)
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	defer s.Close()
	fmt.Println("MySQL 表结构已就绪（cloud_configs、systems）。")
}
