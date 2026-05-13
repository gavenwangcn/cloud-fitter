package huaweicloudregion

import "time"

const (
	// TransientNetworkRetryMaxAttempts 为单次对外 API 调用的总次数（含首次）；再试 2 次即共 3 次。
	TransientNetworkRetryMaxAttempts = 3
)

// DoWithTransientNetworkRetry 对 TransientHuaweiNetworkErr 为 true 的错误做有限次重试（间隔递增，单次间隔上限 2s）。
// 用于各产品 SDK（ECS/RDS/DCS/DMS/CCE/EIP/ELB 等）在 Docker DNS 偶发 no such host 时的统一退避。
func DoWithTransientNetworkRetry[T any](op func() (T, error)) (T, error) {
	var zero T
	var lastErr error
	base := 400 * time.Millisecond
	for attempt := 1; attempt <= TransientNetworkRetryMaxAttempts; attempt++ {
		v, err := op()
		if err == nil {
			return v, nil
		}
		lastErr = err
		if attempt == TransientNetworkRetryMaxAttempts || !TransientHuaweiNetworkErr(err) {
			break
		}
		d := time.Duration(attempt) * base
		if d > 2*time.Second {
			d = 2 * time.Second
		}
		time.Sleep(d)
	}
	return zero, lastErr
}
