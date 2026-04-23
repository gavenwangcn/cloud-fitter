package envtags

import (
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
