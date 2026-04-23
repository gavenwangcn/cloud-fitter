package ecs

import (
	"context"
	"sync"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbecs"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/server/scope"
	"github.com/cloud-fitter/cloud-fitter/internal/service/ecser"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

func ListDetail(ctx context.Context, req *pbecs.ListDetailReq) (*pbecs.ListDetailResp, error) {
	var (
		ecs ecser.Ecser
	)

	tenanters, err := tenanter.GetTenanters(req.Provider)
	if err != nil {
		return nil, errors.WithMessage(err, "getTenanters error")
	}

	region, err := tenanter.NewRegion(req.Provider, req.RegionId)
	if err != nil {
		return nil, errors.WithMessagef(err, "provider %v regionId %v", req.Provider, req.RegionId)
	}

	for _, tenanter := range tenanters {
		if req.AccountName == "" || tenanter.AccountName() == req.AccountName {
			if ecs, err = ecser.NewEcsClient(req.Provider, region, tenanter); err != nil {
				return nil, errors.WithMessage(err, "NewEcsClient error")
			}
			break
		}
	}

	return ecs.ListDetail(ctx, req)
}

func List(ctx context.Context, req *pbecs.ListReq) (*pbecs.ListResp, error) {
	var (
		wg    sync.WaitGroup
		mutex sync.Mutex
		ecses []*pbecs.EcsInstance
	)

	pname := pbtenant.CloudProvider_name[int32(req.Provider)]
	if pname == "" {
		pname = req.Provider.String()
	}

	tenanters, err := tenanter.GetTenanters(req.Provider)
	if err != nil {
		glog.Warningf("ecs List provider=%s: no tenants (%v); skip", pname, err)
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
		return nil, errors.Errorf("no tenants for provider %s account %q", pname, scope.AccountName(ctx))
	}

	regions := tenanter.GetAllRegionIds(req.Provider)
	glog.Infof("ecs List provider=%s accounts=%d regions=%d (goroutines=%d)", pname, len(tenanters), len(regions), len(tenanters)*len(regions))

	wg.Add(len(tenanters) * len(regions))
	for _, t := range tenanters {
		for _, region := range regions {
			go func(tenant tenanter.Tenanter, region tenanter.Region) {
				defer wg.Done()
				rName := region.GetName()
				acc := tenant.AccountName()
				rid := region.GetId()
				ecs, err := ecser.NewEcsClient(req.Provider, region, tenant)
				if err != nil {
					glog.Errorf("ecs provider=%s region=%s(regionId=%d) account=%s NewEcsClient failed: %v", pname, rName, rid, acc, err)
					return
				}
				glog.Infof("ecs provider=%s region=%s(regionId=%d) account=%s client ok, start ListDetail pagination", pname, rName, rid, acc)

				var regionTotal int
				request := &pbecs.ListDetailReq{
					Provider:    req.Provider,
					AccountName: acc,
					RegionId:    rid,
					PageNumber:  1,
					PageSize:    100,
					NextToken:   "",
				}
				page := 0
				for {
					page++
					resp, err := ecs.ListDetail(ctx, request)
					if err != nil {
						glog.Errorf("ecs provider=%s region=%s(regionId=%d) account=%s ListDetail page=%d failed (no data merged): %v", pname, rName, rid, acc, page, err)
						return
					}
					n := len(resp.Ecses)
					regionTotal += n
					mutex.Lock()
					ecses = append(ecses, resp.Ecses...)
					mutex.Unlock()
					glog.Infof("ecs provider=%s region=%s account=%s ListDetail page=%d batch=%d region_total_so_far=%d finished=%v", pname, rName, acc, page, n, regionTotal, resp.Finished)
					if resp.Finished {
						break
					}
					request.PageNumber, request.PageSize, request.NextToken = resp.PageNumber, resp.PageSize, resp.NextToken
				}
				if regionTotal == 0 {
					glog.Warningf("ecs provider=%s region=%s(regionId=%d) account=%s list finished pages=%d but instances_in_region=0 (API ok but empty; check region/account/IAM)", pname, rName, rid, acc, page)
				}
				glog.Infof("ecs provider=%s region=%s account=%s list ok pages=%d instances_in_region=%d", pname, rName, acc, page, regionTotal)
			}(t, region)

		}
	}
	wg.Wait()

	glog.Infof("ecs List provider=%s done total_instances=%d", pname, len(ecses))
	return &pbecs.ListResp{Ecses: ecses}, nil
}

func ListAll(ctx context.Context) (*pbecs.ListResp, error) {
	var (
		wg    sync.WaitGroup
		mutex sync.Mutex
		ecses []*pbecs.EcsInstance
	)

	glog.Infof("ecs ListAll: aggregating all providers (%d)", len(pbtenant.CloudProvider_name))
	wg.Add(len(pbtenant.CloudProvider_name))
	for k := range pbtenant.CloudProvider_name {
		go func(provider int32) {
			defer wg.Done()

			pname := pbtenant.CloudProvider_name[provider]
			resp, err := List(ctx, &pbecs.ListReq{Provider: pbtenant.CloudProvider(provider)})
			if err != nil {
				glog.Warningf("ecs ListAll provider=%s: %v", pname, err)
				return
			}

			mutex.Lock()
			n := len(resp.Ecses)
			ecses = append(ecses, resp.Ecses...)
			mutex.Unlock()
			glog.Infof("ecs ListAll provider=%s instances=%d", pname, n)
		}(k)
	}

	wg.Wait()

	glog.Infof("ecs ListAll finished total_instances=%d", len(ecses))
	return &pbecs.ListResp{Ecses: ecses}, nil
}
