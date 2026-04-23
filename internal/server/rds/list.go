package rds

import (
	"context"
	"sync"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbrds"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/service/rdser"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

func ListDetail(ctx context.Context, req *pbrds.ListDetailReq) (*pbrds.ListDetailResp, error) {
	var (
		rds rdser.Rdser
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
			if rds, err = rdser.NewRdsClient(req.Provider, region, tenanter); err != nil {
				return nil, errors.WithMessage(err, "NewRdsClient error")
			}
			break
		}
	}

	return rds.ListDetail(ctx, req)
}

func List(ctx context.Context, req *pbrds.ListReq) (*pbrds.ListResp, error) {
	var (
		wg    sync.WaitGroup
		mutex sync.Mutex
		rdses []*pbrds.RdsInstance
	)

	tenanters, err := tenanter.GetTenanters(req.Provider)
	if err != nil {
		return nil, errors.WithMessage(err, "getTenanters error")
	}

	regions := tenanter.GetAllRegionIds(req.Provider)

	wg.Add(len(tenanters) * len(regions))
	for _, t := range tenanters {
		for _, region := range regions {
			go func(tenant tenanter.Tenanter, region tenanter.Region) {
				defer wg.Done()
				rds, err := rdser.NewRdsClient(req.Provider, region, tenant)
				if err != nil {
					glog.Errorf("New Rds Client error %v", err)
					return
				}

				request := &pbrds.ListDetailReq{
					Provider:    req.Provider,
					AccountName: tenant.AccountName(),
					RegionId:    region.GetId(),
					PageNumber:  1,
					PageSize:    100,
					NextToken:   "",
				}
				for {
					resp, err := rds.ListDetail(ctx, request)
					if err != nil {
						glog.Errorf("ListDetail error %v", err)
						return
					}
					mutex.Lock()
					rdses = append(rdses, resp.Rdses...)
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

	return &pbrds.ListResp{Rdses: rdses}, nil
}

func ListAll(ctx context.Context) (*pbrds.ListResp, error) {
	var (
		wg    sync.WaitGroup
		mutex sync.Mutex
		rdses []*pbrds.RdsInstance
	)

	glog.Infof("rds ListAll: aggregating all providers (%d)", len(pbtenant.CloudProvider_name))
	wg.Add(len(pbtenant.CloudProvider_name))
	for k := range pbtenant.CloudProvider_name {
		go func(provider int32) {
			defer wg.Done()

			pname := pbtenant.CloudProvider_name[provider]
			resp, err := List(ctx, &pbrds.ListReq{Provider: pbtenant.CloudProvider(provider)})
			if err != nil {
				glog.Warningf("rds ListAll provider=%s: %v", pname, err)
				return
			}

			mutex.Lock()
			n := len(resp.Rdses)
			rdses = append(rdses, resp.Rdses...)
			mutex.Unlock()
			glog.Infof("rds ListAll provider=%s instances=%d", pname, n)
		}(k)
	}

	wg.Wait()

	glog.Infof("rds ListAll finished total_instances=%d", len(rdses))
	return &pbrds.ListResp{Rdses: rdses}, nil
}
