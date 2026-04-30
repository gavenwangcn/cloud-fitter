package elb

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/golang/glog"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hweip "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/eip/v2"
	eipmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/eip/v2/model"
	hwelb "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3"
	elbmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3/model"
	hwiam "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3"
	iammodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3/model"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweicloudregion"
	"github.com/cloud-fitter/cloud-fitter/internal/server/scope"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

type Instance struct {
	Provider          string `json:"provider"`
	AccountName       string `json:"accountName"`
	RegionName        string `json:"regionName"`
	ID                string `json:"id"`
	Name              string `json:"name"`
	InstanceType      string `json:"instanceType"`
	IPv4Private       string `json:"ipv4PrivateAddress"`
	IPv4Public        string `json:"ipv4PublicAddress"`
	Listeners         string `json:"listeners"`
	IPv4BandwidthMbit int32  `json:"ipv4BandwidthMbit"`
	OnlineTime        string `json:"onlineTime"`
	Status            string `json:"status"`
	VpcID             string `json:"vpcId"`
}

func List(ctx context.Context, provider pbtenant.CloudProvider) ([]*Instance, error) {
	if provider != pbtenant.CloudProvider_huawei {
		return nil, nil
	}
	tenanters, err := tenanter.GetTenanters(provider)
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
		return nil, errors.Errorf("no tenants for provider %v account %q", provider, scope.AccountName(ctx))
	}
	regions := tenanter.GetAllRegionIds(provider)

	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		all []*Instance
	)
	wg.Add(len(tenanters) * len(regions))
	for _, t := range tenanters {
		for _, r := range regions {
			go func(tenant tenanter.Tenanter, region tenanter.Region) {
				defer wg.Done()
				items, err := listHuaweiElbByRegion(tenant, region)
				if err != nil {
					glog.Errorf("elb list failed account=%s region=%s err=%v", tenant.AccountName(), region.GetName(), err)
					return
				}
				mu.Lock()
				all = append(all, items...)
				mu.Unlock()
			}(t, r)
		}
	}
	wg.Wait()
	return all, nil
}

func listHuaweiElbByRegion(tenant tenanter.Tenanter, region tenanter.Region) ([]*Instance, error) {
	t, ok := tenant.(*tenanter.AccessKeyTenant)
	if !ok {
		return nil, nil
	}
	rName := region.GetName()
	baseAuth := basic.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).Build()
	iamHc := hwiam.IamClientBuilder().
		WithRegion(huaweicloudregion.EndpointForService("iam", rName)).
		WithCredential(baseAuth).
		Build()
	iamCli := hwiam.NewIamClient(iamHc)
	projReq := new(iammodel.KeystoneListProjectsRequest)
	projReq.Name = &rName
	projResp, err := iamCli.KeystoneListProjects(projReq)
	if err != nil || projResp == nil || projResp.Projects == nil || len(*projResp.Projects) == 0 {
		return nil, errors.Wrapf(err, "Huawei KeystoneListProjects regionName %s", rName)
	}
	projectID := (*projResp.Projects)[0].Id

	auth := basic.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).WithProjectId(projectID).Build()
	elbCli := hwelb.NewElbClient(hwelb.ElbClientBuilder().
		WithRegion(huaweicloudregion.EndpointForService("elb", rName)).
		WithCredential(auth).
		Build())
	eipCli := hweip.NewEipClient(hweip.EipClientBuilder().
		WithRegion(huaweicloudregion.EndpointForService("eip", rName)).
		WithCredential(auth).
		Build())

	eipBandwidth := make(map[string]int32)
	eipReq := new(eipmodel.ListPublicipsRequest)
	eipLimit := int32(2000)
	eipReq.Limit = &eipLimit
	if eipResp, e := eipCli.ListPublicips(eipReq); e == nil && eipResp != nil && eipResp.Publicips != nil {
		for _, ip := range *eipResp.Publicips {
			id := strings.TrimSpace(deref(ip.Id))
			addr := strings.TrimSpace(deref(ip.PublicIpAddress))
			bw := derefI32(ip.BandwidthSize)
			if id != "" {
				eipBandwidth[id] = bw
			}
			if addr != "" {
				eipBandwidth[addr] = bw
			}
		}
	}

	var out []*Instance
	req := new(elbmodel.ListLoadBalancersRequest)
	limit := int32(200)
	req.Limit = &limit
	for {
		resp, err := elbCli.ListLoadBalancers(req)
		if err != nil {
			return nil, errors.Wrap(err, "Huawei ELB ListLoadBalancers error")
		}
		if resp != nil && resp.Loadbalancers != nil {
			for _, lb := range *resp.Loadbalancers {
				publicIP, bandwidth := pickPublicIPAndBandwidth(lb, eipBandwidth)
				listeners, _ := queryListeners(elbCli, lb.Id)
				out = append(out, &Instance{
					Provider:          "huawei",
					AccountName:       tenant.AccountName(),
					RegionName:        rName,
					ID:                lb.Id,
					Name:              lb.Name,
					InstanceType:      elbTypeText(lb),
					IPv4Private:       lb.VipAddress,
					IPv4Public:        publicIP,
					Listeners:         strings.Join(listeners, "，"),
					IPv4BandwidthMbit: bandwidth,
					OnlineTime:        lb.CreatedAt,
					Status:            fmt.Sprint(lb.OperatingStatus),
					VpcID:             lb.VpcId,
				})
			}
		}
		if resp == nil || resp.PageInfo == nil || resp.PageInfo.NextMarker == nil || strings.TrimSpace(*resp.PageInfo.NextMarker) == "" {
			break
		}
		req.Marker = resp.PageInfo.NextMarker
	}
	return out, nil
}

func queryListeners(cli *hwelb.ElbClient, lbID string) ([]string, error) {
	req := new(elbmodel.ListListenersRequest)
	req.Limit = int32Ptr(200)
	req.LoadbalancerId = &[]string{lbID}
	var out []string
	for {
		resp, err := cli.ListListeners(req)
		if err != nil {
			return out, err
		}
		if resp != nil && resp.Listeners != nil {
			for _, l := range *resp.Listeners {
				out = append(out, fmt.Sprintf("%s/%d", l.Protocol, l.ProtocolPort))
			}
		}
		if resp == nil || resp.PageInfo == nil || resp.PageInfo.NextMarker == nil || strings.TrimSpace(*resp.PageInfo.NextMarker) == "" {
			break
		}
		req.Marker = resp.PageInfo.NextMarker
	}
	return out, nil
}

func pickPublicIPAndBandwidth(lb elbmodel.LoadBalancer, eipBW map[string]int32) (string, int32) {
	var ip string
	var bw int32
	if lb.Publicips != nil {
		for _, p := range *lb.Publicips {
			if strings.TrimSpace(p.PublicipAddress) == "" {
				continue
			}
			ip = p.PublicipAddress
			if v, ok := eipBW[p.PublicipId]; ok && v > bw {
				bw = v
			}
			if v, ok := eipBW[p.PublicipAddress]; ok && v > bw {
				bw = v
			}
			break
		}
	}
	if ip == "" {
		for _, e := range lb.Eips {
			if strings.TrimSpace(deref(e.EipAddress)) == "" {
				continue
			}
			ip = deref(e.EipAddress)
			if v, ok := eipBW[deref(e.EipId)]; ok && v > bw {
				bw = v
			}
			if v, ok := eipBW[ip]; ok && v > bw {
				bw = v
			}
			break
		}
	}
	return ip, bw
}

func elbTypeText(lb elbmodel.LoadBalancer) string {
	if lb.Guaranteed {
		return "独享型"
	}
	return "共享型"
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefI32(n *int32) int32 {
	if n == nil {
		return 0
	}
	return *n
}

func int32Ptr(v int32) *int32 { return &v }
