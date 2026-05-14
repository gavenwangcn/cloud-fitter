package resourcecache

import (
	"strings"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/google/uuid"
)

// 与 internal/cmdb/sync.go 中 parseCloudInstanceUUID / middlewareCMDBUUID 对齐，供快照表主键与 CMDB reconcile 一致。

func parseCloudInstanceUUID(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := uuid.Parse(raw)
	if err != nil {
		return "", false
	}
	return u.String(), true
}

var namespaceMiddlewareNonRFC = uuid.MustParse("8f7e6d5c-4b3a-2918-0f1e-2d3c4b5a6978")

func middlewareCMDBUUID(instanceID string) (string, bool) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return "", false
	}
	if u, err := uuid.Parse(instanceID); err == nil {
		return u.String(), true
	}
	u := uuid.NewSHA1(namespaceMiddlewareNonRFC, []byte(instanceID))
	return u.String(), true
}

func cloudTypeLabel(p pbtenant.CloudProvider) string {
	switch p {
	case pbtenant.CloudProvider_huawei:
		return "华为云"
	case pbtenant.CloudProvider_ali:
		return "阿里云"
	case pbtenant.CloudProvider_tencent:
		return "腾讯云"
	case pbtenant.CloudProvider_aws:
		return "AWS"
	default:
		return "云"
	}
}

func effectiveSysNodeName(p pbtenant.CloudProvider, region, nodeTagValue string) string {
	baseCloud := cloudTypeLabel(p)
	region = strings.TrimSpace(region)
	s := strings.TrimSpace(nodeTagValue)
	base := ""
	if region != "" {
		base = baseCloud + "-" + region
	} else if baseCloud != "" {
		base = baseCloud
	}
	if s != "" && base != "" && strings.HasPrefix(s, base) {
		return s
	}
	if s != "" && base != "" {
		return base + "-" + s
	}
	if s != "" {
		return s
	}
	return base
}

// MiddlewareResourceKey 与 CMDB middlewareCMDBUUID 一致。
func MiddlewareResourceKey(instanceID string) (string, bool) {
	return middlewareCMDBUUID(instanceID)
}

// SysNodeKey 与 CMDB 中 effectiveSysNodeName 一致，写入快照 sys_node_key。
func SysNodeKey(p pbtenant.CloudProvider, region, nodeTag string) string {
	return effectiveSysNodeName(p, region, nodeTag)
}

// ParseCloudUUID 解析云实例 ID 为规范 UUID（与 CMDB parseCloudInstanceUUID 一致）。
func ParseCloudUUID(raw string) (string, bool) {
	return parseCloudInstanceUUID(raw)
}

// ResourceKeyECS 快照表主键列 resource_key：合法 UUID 则规范化，否则截断原始实例 ID。
func ResourceKeyECS(instanceID string) string {
	if u, ok := parseCloudInstanceUUID(instanceID); ok {
		return u
	}
	s := strings.TrimSpace(instanceID)
	if len(s) > 128 {
		s = s[:128]
	}
	return s
}
