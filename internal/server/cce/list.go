package cce

import (
	"context"
	"sync"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbcce"
	"github.com/cloud-fitter/cloud-fitter/internal/server/scope"
	"github.com/cloud-fitter/cloud-fitter/internal/service/ccer"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

func ListDetail(ctx context.Context, req *pbcce.ListDetailReq) (*pbcce.ListDetailResp, error) {
	var cli ccer.Ccer

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
			if cli, err = ccer.NewCceClient(req.Provider, region, tn); err != nil {
				return nil, errors.WithMessage(err, "NewCceClient error")
			}
			break
		}
	}

	if cli == nil {
		return nil, errors.Errorf("no cce client for provider %v account %q", req.Provider, req.AccountName)
	}

	return cli.ListDetail(ctx, req)
}

func List(ctx context.Context, req *pbcce.ListReq) (*pbcce.ListResp, error) {
	var (
		wg       sync.WaitGroup
		mutex    sync.Mutex
		clusters []*pbcce.CceCluster
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
				cli, err := ccer.NewCceClient(req.Provider, region, tenant)
				if err != nil {
					glog.Errorf("NewCceClient error %v", err)
					return
				}

				request := &pbcce.ListDetailReq{
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
						glog.Errorf("cce ListDetail error %v", err)
						return
					}
					mutex.Lock()
					clusters = append(clusters, resp.Clusters...)
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

	return &pbcce.ListResp{Clusters: clusters}, nil
}
