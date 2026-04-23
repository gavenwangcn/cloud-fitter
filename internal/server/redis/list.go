package redis

import (
	"context"
	"sync"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbredis"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/server/scope"
	"github.com/cloud-fitter/cloud-fitter/internal/service/rediser"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

func ListDetail(ctx context.Context, req *pbredis.ListDetailReq) (*pbredis.ListDetailResp, error) {
	var (
		cli rediser.Rediser
	)

	tenanters, err := tenanter.GetTenanters(req.Provider)
	if err != nil {
		return nil, errors.WithMessage(err, "getTenanters error")
	}

	region, err := tenanter.NewRegion(req.Provider, req.RegionId)
	if err != nil {
		return nil, errors.WithMessagef(err, "provider %v regionId %v", req.Provider, req.RegionId)
	}

	for _, tn := range tenanters {
		if req.AccountName == "" || tn.AccountName() == req.AccountName {
			if cli, err = rediser.NewRedisClient(req.Provider, region, tn); err != nil {
				return nil, errors.WithMessage(err, "NewRedisClient error")
			}
			break
		}
	}

	return cli.ListDetail(ctx, req)
}

func List(ctx context.Context, req *pbredis.ListReq) (*pbredis.ListResp, error) {
	var (
		wg       sync.WaitGroup
		mutex    sync.Mutex
		redises  []*pbredis.RedisInstance
	)

	tenanters, err := tenanter.GetTenanters(req.Provider)
	if err != nil {
		return nil, errors.WithMessage(err, "getTenanters error")
	}

	if acc := scope.AccountName(ctx); acc != "" {
		var filtered []tenanter.Tenanter
		for _, t := range tenanters {
			if t.AccountName() == acc {
				filtered = append(filtered, t)
			}
		}
		tenanters = filtered
	}
	if len(tenanters) == 0 {
		return nil, errors.Errorf("no tenants for provider %v account %q", req.Provider, scope.AccountName(ctx))
	}

	regions := tenanter.GetAllRegionIds(req.Provider)

	wg.Add(len(tenanters) * len(regions))
	for _, t := range tenanters {
		for _, region := range regions {
			go func(tenant tenanter.Tenanter, region tenanter.Region) {
				defer wg.Done()
				cli, err := rediser.NewRedisClient(req.Provider, region, tenant)
				if err != nil {
					glog.Errorf("New Redis Client error %v", err)
					return
				}

				request := &pbredis.ListDetailReq{
					Provider:    req.Provider,
					AccountName: tenant.AccountName(),
					RegionId:    region.GetId(),
					PageNumber:  1,
					PageSize:    100,
					NextToken:   "",
				}
				for {
					resp, err := cli.ListDetail(ctx, request)
					if err != nil {
						glog.Errorf("ListDetail error %v", err)
						return
					}
					mutex.Lock()
					redises = append(redises, resp.Redises...)
					mutex.Unlock()
					if resp.Finished {
						break
					}
					request.PageNumber, request.PageSize, request.NextToken = resp.PageNumber, resp.PageSize, resp.NextToken
				}
			}(t, region)

		}
	}
	wg.Wait()

	return &pbredis.ListResp{Redises: redises}, nil
}

func ListAll(ctx context.Context) (*pbredis.ListResp, error) {
	var (
		wg      sync.WaitGroup
		mutex   sync.Mutex
		redises []*pbredis.RedisInstance
	)

	glog.Infof("redis ListAll: aggregating all providers (%d)", len(pbtenant.CloudProvider_name))
	wg.Add(len(pbtenant.CloudProvider_name))
	for k := range pbtenant.CloudProvider_name {
		go func(provider int32) {
			defer wg.Done()

			pname := pbtenant.CloudProvider_name[provider]
			resp, err := List(ctx, &pbredis.ListReq{Provider: pbtenant.CloudProvider(provider)})
			if err != nil {
				glog.Warningf("redis ListAll provider=%s: %v", pname, err)
				return
			}

			mutex.Lock()
			n := len(resp.Redises)
			redises = append(redises, resp.Redises...)
			mutex.Unlock()
			glog.Infof("redis ListAll provider=%s instances=%d", pname, n)
		}(k)
	}

	wg.Wait()

	glog.Infof("redis ListAll finished total_instances=%d", len(redises))
	return &pbredis.ListResp{Redises: redises}, nil
}
