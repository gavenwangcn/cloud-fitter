package cmdb

import (
	"context"
	"sync"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/configstore"
	"github.com/cloud-fitter/cloud-fitter/internal/service/ccer"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

// huaweiEcsIDToClusterUIDMap 在「与 jsonapi 相同的系统账号与区域维度」上，使用华为 CCE
// ListClusters + ListNodes（见 ccer.HuaweiCce.MapEcsInstanceIDToClusterUID）建立 ECS 实例 ID 到集群 UID 的映射。
// 非华为云账号被跳过；多账号、多区域结果合并为一张表。
func huaweiEcsIDToClusterUIDMap(ctx context.Context, store *configstore.Store, systemName string) (map[string]string, error) {
	accounts, err := store.AccountsBySystemName(systemName)
	if err != nil {
		return nil, errors.WithMessage(err, "AccountsBySystemName")
	}
	tenantersHuawei, err := tenanter.GetTenanters(pbtenant.CloudProvider_huawei)
	if err != nil {
		return nil, errors.WithMessage(err, "GetTenanters huawei")
	}
	regions := tenanter.GetAllRegionIds(pbtenant.CloudProvider_huawei)
	merged := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, acc := range accounts {
		if acc.Provider != int32(pbtenant.CloudProvider_huawei) {
			continue
		}
		var tenant tenanter.Tenanter
		for _, t := range tenantersHuawei {
			if t.AccountName() == acc.Name {
				tenant = t
				break
			}
		}
		if tenant == nil {
			glog.Warningf("cmdb: huawei account %q not in loaded tenanters, skip cce map", acc.Name)
			continue
		}
		for _, region := range regions {
			wg.Add(1)
			go func(reg tenanter.Region, tn tenanter.Tenanter) {
				defer wg.Done()
				cli, err := ccer.NewCceClient(pbtenant.CloudProvider_huawei, reg, tn)
				if err != nil {
					glog.V(1).Infof("cmdb: NewCceClient account=%s region=%s: %v", tn.AccountName(), reg.GetName(), err)
					return
				}
				m, err := cli.MapEcsInstanceIDToClusterUID(ctx)
				if err != nil {
					glog.Warningf("cmdb: MapEcsInstanceIDToClusterUID account=%s region=%s: %v", tn.AccountName(), reg.GetName(), err)
					return
				}
				if len(m) == 0 {
					return
				}
				mu.Lock()
				for k, v := range m {
					merged[k] = v
				}
				mu.Unlock()
			}(region, tenant)
		}
	}
	wg.Wait()
	return merged, nil
}
