package envtags

import (
	"strings"

	alirds "github.com/aliyun/alibaba-cloud-sdk-go/services/rds"
)

// AliRDSInstanceTagMap 调用 DescribeTags（不传 DBInstanceId）得到区域内标签与实例关系，再按 wantKey 建 instanceId -> TagValue。
func AliRDSInstanceTagMap(cli *alirds.Client, wantKey string) (map[string]string, error) {
	if wantKey == "" || cli == nil {
		return nil, nil
	}
	req := alirds.CreateDescribeTagsRequest()
	resp, err := cli.DescribeTags(req)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string)
	wantKey = strings.TrimSpace(wantKey)
	for _, ti := range resp.Items.TagInfos {
		if !strings.EqualFold(strings.TrimSpace(ti.TagKey), wantKey) {
			continue
		}
		val := strings.TrimSpace(ti.TagValue)
		for _, id := range ti.DBInstanceIds.DBInstanceIds {
			id = strings.TrimSpace(id)
			if id != "" {
				out[id] = val
			}
		}
	}
	return out, nil
}
