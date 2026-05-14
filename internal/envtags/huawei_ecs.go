package envtags

import (
	"fmt"
	"strings"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
)

// HuaweiECSFromServerDetail 从华为 ECS ListServersDetails 单条 ServerDetail 解析标签值。
func HuaweiECSFromServerDetail(v model.ServerDetail, wantKey string) string {
	if wantKey == "" {
		return ""
	}
	if v.SysTags != nil {
		var pairs [][2]string
		for _, st := range *v.SysTags {
			if st.Key != nil && st.Value != nil {
				pairs = append(pairs, [2]string{*st.Key, *st.Value})
			}
		}
		if s := FromPairs(wantKey, pairs); s != "" {
			return s
		}
	}
	if v.Tags != nil {
		for _, raw := range *v.Tags {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			i := strings.IndexByte(raw, ',')
			if i <= 0 || i >= len(raw)-1 {
				continue
			}
			k := strings.TrimSpace(raw[:i])
			vl := strings.TrimSpace(raw[i+1:])
			if strings.EqualFold(k, wantKey) {
				return vl
			}
		}
	}
	return ""
}

// HuaweiECSUserTagPairsFromServerDetail 从 ListServersDetails 的 Tags（逗号分隔 key,value 字符串）解析用户标签键值。
func HuaweiECSUserTagPairsFromServerDetail(v model.ServerDetail) [][2]string {
	if v.Tags == nil {
		return nil
	}
	var out [][2]string
	for _, raw := range *v.Tags {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		i := strings.IndexByte(raw, ',')
		if i <= 0 || i >= len(raw)-1 {
			continue
		}
		k := strings.TrimSpace(raw[:i])
		vl := strings.TrimSpace(raw[i+1:])
		if k != "" {
			out = append(out, [2]string{k, vl})
		}
	}
	return out
}

// HuaweiECSTagPairsFromShowServerTags 将「查询云服务器标签」ShowServerTags 响应中的 tags 转为 (key,value) 列表。
// 与华为云 API 文档一致：每项含 key、value；键名匹配使用 FromPairs（大小写不敏感）。
func HuaweiECSTagPairsFromShowServerTags(tags *[]model.ServerTag) [][2]string {
	if tags == nil {
		return nil
	}
	var out [][2]string
	for _, t := range *tags {
		k := strings.TrimSpace(t.Key)
		if k == "" {
			continue
		}
		v := ""
		if t.Value != nil {
			v = strings.TrimSpace(*t.Value)
		}
		out = append(out, [2]string{k, v})
	}
	return out
}

// HuaweiECSListDetailTagsSummary 摘要 ListServersDetails 单条里的 sys_tags / tags，便于与 ShowServerTags 对照排查。
func HuaweiECSListDetailTagsSummary(v model.ServerDetail) string {
	var sb strings.Builder
	if v.SysTags != nil {
		sb.WriteString(fmt.Sprintf("sys_tags=%d", len(*v.SysTags)))
	} else {
		sb.WriteString("sys_tags=nil")
	}
	if v.Tags != nil {
		n := len(*v.Tags)
		sb.WriteString(fmt.Sprintf(";tags_str_list=%d", n))
		for i, raw := range *v.Tags {
			if i >= 3 {
				sb.WriteString(";tags_sample=…(trunc)")
				break
			}
			if i == 0 {
				sb.WriteString(";tags_sample=")
			} else {
				sb.WriteString("|")
			}
			sb.WriteString(strings.TrimSpace(raw))
		}
	} else {
		sb.WriteString(";tags_str_list=nil")
	}
	return sb.String()
}
