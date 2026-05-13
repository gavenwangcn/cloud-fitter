package huaweicloudregion

import (
	"context"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/config"
)

// SDKHttpConfig 返回华为云 Go SDK 的 HTTP 配置，用于缓解 Docker 内置 DNS（127.0.0.11）
// 或弱网环境下偶发的解析/建连超时。请在各 *ClientBuilder 链上 .WithHttpConfig(SDKHttpConfig())。
//
// 环境变量（可选，单位：秒）：
//   HUAWEI_SDK_HTTP_TIMEOUT_SECS — 单次请求总超时，默认 240
//   HUAWEI_SDK_DIAL_TIMEOUT_SECS — net.Dialer 的 Timeout（含 DNS 解析 + TCP 建连），默认 60
//
// 若日志为 no such host / NXDOMAIN，属于解析失败而非「超时过短」，应修正容器 DNS（compose 的 dns:）
// 或网络出口；仅靠调大本处秒数无法解决「域名不存在」类错误。
func SDKHttpConfig() *config.HttpConfig {
	c := config.DefaultHttpConfig()

	httpSecs := 240
	if s := os.Getenv("HUAWEI_SDK_HTTP_TIMEOUT_SECS"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			httpSecs = v
		}
	}
	c = c.WithTimeout(time.Duration(httpSecs) * time.Second)

	dialSecs := 60
	if s := os.Getenv("HUAWEI_SDK_DIAL_TIMEOUT_SECS"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			dialSecs = v
		}
	}
	d := net.Dialer{
		Timeout:   time.Duration(dialSecs) * time.Second,
		KeepAlive: 30 * time.Second,
	}
	return c.WithDialContext(func(ctx context.Context, network, addr string) (net.Conn, error) {
		return d.DialContext(ctx, network, addr)
	})
}
