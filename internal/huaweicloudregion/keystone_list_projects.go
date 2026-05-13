package huaweicloudregion

import (
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	hwiam "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3"
	iammodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3/model"
)

// keystoneProjectLookupMaxAttempts 为 IAM 按地域查 project 的 HTTP 调用总次数（含首次）。
// 默认 4：首次失败后最多再试 3 次。环境变量 HUAWEI_IAM_PROJECT_LOOKUP_RETRIES，范围 1～20。
func keystoneProjectLookupMaxAttempts() int {
	const def = 4
	s := strings.TrimSpace(os.Getenv("HUAWEI_IAM_PROJECT_LOOKUP_RETRIES"))
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return def
	}
	if n > 20 {
		return 20
	}
	return n
}

// keystoneProjectLookupBaseDelay 为重试前 Sleep 的基数（毫秒），默认 800；环境变量 HUAWEI_IAM_PROJECT_LOOKUP_RETRY_BASE_MS，范围 50～5000。
func keystoneProjectLookupBaseDelay() time.Duration {
	const defMs = 800
	s := strings.TrimSpace(os.Getenv("HUAWEI_IAM_PROJECT_LOOKUP_RETRY_BASE_MS"))
	if s == "" {
		return time.Duration(defMs) * time.Millisecond
	}
	ms, err := strconv.Atoi(s)
	if err != nil || ms < 50 {
		return time.Duration(defMs) * time.Millisecond
	}
	if ms > 5000 {
		return 5 * time.Second
	}
	return time.Duration(ms) * time.Millisecond
}

// TransientHuaweiNetworkErr 判断华为 SDK 请求失败是否可能为瞬时网络/DNS 问题（适合有限次重试）。
// IAM KeystoneListProjects、CCE/ECS 等对外域名解析均可能命中 Docker 127.0.0.11 偶发失败。
func TransientHuaweiNetworkErr(err error) bool {
	if err == nil {
		return false
	}
	var ne *net.OpError
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "temporary failure in name resolution") ||
		strings.Contains(msg, "server misbehaving")
}

// KeystoneListProjectsResolveProject 按 region 名称查询 IAM project（与现有逻辑一致），
// 对 DNS/连接类瞬时错误做有限次重试，减轻 Docker 127.0.0.11 偶发 no such host / timeout。
func KeystoneListProjectsResolveProject(client *hwiam.IamClient, regionName string) (*iammodel.KeystoneListProjectsResponse, error) {
	max := keystoneProjectLookupMaxAttempts()
	base := keystoneProjectLookupBaseDelay()
	var lastErr error
	for attempt := 1; attempt <= max; attempt++ {
		req := new(iammodel.KeystoneListProjectsRequest)
		req.Name = &regionName
		r, err := client.KeystoneListProjects(req)
		if err != nil {
			lastErr = err
			if attempt < max && TransientHuaweiNetworkErr(err) {
				d := time.Duration(attempt) * base
				if d > 3*time.Second {
					d = 3 * time.Second
				}
				time.Sleep(d)
				continue
			}
			break
		}
		if r == nil || r.Projects == nil || len(*r.Projects) == 0 {
			return r, errors.New("empty project list")
		}
		return r, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("keystone list projects: unknown failure")
}
