package ccer

import (
	"context"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbcce"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

var (
	ErrCceListNotSupported = errors.New("cloud not supported cce list")
	ErrCcerPanic           = errors.New("cce client init panic")
)

type Ccer interface {
	ListDetail(ctx context.Context, req *pbcce.ListDetailReq) (*pbcce.ListDetailResp, error)
	// MapEcsInstanceIDToClusterUID 云厂商若支持，则返回 CCE/托管集群中节点对应 ECS 实例 ID 到集群资源 ID 的映射；华为为 metadata.uid 与 ListNodes 路径中的 cluster_id 一致。不支持时返回 (nil, nil) 或空 map。
	MapEcsInstanceIDToClusterUID(ctx context.Context) (map[string]string, error)
}

func NewCceClient(provider pbtenant.CloudProvider, region tenanter.Region, tenant tenanter.Tenanter) (ccer Ccer, err error) {
	defer func() {
		if err1 := recover(); err1 != nil {
			glog.Errorf("NewCceClient panic %v", err1)
			err = errors.WithMessagef(ErrCcerPanic, "%v", err1)
		}
	}()

	switch provider {
	case pbtenant.CloudProvider_huawei:
		return newHuaweiCceClient(region, tenant)
	}

	err = errors.WithMessagef(ErrCceListNotSupported, "cloud provider %v region %v", provider, region)
	return
}
