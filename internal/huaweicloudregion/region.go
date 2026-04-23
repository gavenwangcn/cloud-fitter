// Package huaweicloudregion 构造华为云 endpoint，避免 huaweicloud-sdk-go-v3 旧版各服务
// region.ValueOf 未收录区域（如 cn-east-4）时 panic。
package huaweicloudregion

import (
	"fmt"

	core "github.com/huaweicloud/huaweicloud-sdk-go-v3/core/region"
)

// EndpointForService 标准公有云域名：https://{service}.{regionID}.myhuaweicloud.com
func EndpointForService(serviceName, regionID string) *core.Region {
	if regionID == "" {
		panic("huaweicloudregion: empty regionID")
	}
	u := fmt.Sprintf("https://%s.%s.myhuaweicloud.com", serviceName, regionID)
	return core.NewRegion(regionID, u)
}
