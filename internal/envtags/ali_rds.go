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

// AliRDSInstanceTagValues 一次 DescribeTags，按 envKey / nodeKey 分别建 instanceId -> 标签值（键名为空则跳过该维度）。
func AliRDSInstanceTagValues(cli *alirds.Client, envKey, nodeKey string) (envByInst, nodeByInst map[string]string, err error) {
	wantEnv := strings.TrimSpace(envKey)
	wantNode := strings.TrimSpace(nodeKey)
	if cli == nil || (wantEnv == "" && wantNode == "") {
		return nil, nil, nil
	}
	req := alirds.CreateDescribeTagsRequest()
	resp, err := cli.DescribeTags(req)
	if err != nil {
		return nil, nil, err
	}
	if wantEnv != "" {
		envByInst = make(map[string]string)
	}
	if wantNode != "" {
		nodeByInst = make(map[string]string)
	}
	for _, ti := range resp.Items.TagInfos {
		tk := strings.TrimSpace(ti.TagKey)
		val := strings.TrimSpace(ti.TagValue)
		if wantEnv != "" && strings.EqualFold(tk, wantEnv) {
			for _, id := range ti.DBInstanceIds.DBInstanceIds {
				id = strings.TrimSpace(id)
				if id != "" {
					envByInst[id] = val
				}
			}
		}
		if wantNode != "" && strings.EqualFold(tk, wantNode) {
			for _, id := range ti.DBInstanceIds.DBInstanceIds {
				id = strings.TrimSpace(id)
				if id != "" {
					nodeByInst[id] = val
				}
			}
		}
	}
	return envByInst, nodeByInst, nil
}
