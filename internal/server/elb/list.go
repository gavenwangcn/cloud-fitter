package elb

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hweip "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/eip/v2"
	eipmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/eip/v2/model"
	hwelbv2 "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v2"
	elbv2model "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v2/model"
	hwelb "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3"
	elbmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3/model"
	hwiam "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/envtags"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweitags"
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
	// 系统标签：ELB 列表接口未区分系统标签时为空
	SystemTagsDisplay string `json:"systemTagsDisplay"`
	// 用户自定义标签：ELB v2 ShowLoadbalancerTags 与 v3 ListLoadBalancers.tags 合并（专用接口优先）
	UserTagsDisplay string `json:"userTagsDisplay"`
	// 环境(标签)：与 ECS/RDS 相同键名，合并标签后 + 负载均衡名称上的名字规则
	EnvTagValue string `json:"envTagValue"`
	// 节点(标签)：「华为云-地域-节点语义」展示串
	NodeTagValue string `json:"nodeTagValue"`
}

func List(ctx context.Context, provider pbtenant.CloudProvider) ([]*Instance, error) {
	begin := time.Now()
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
	nJobs := 0
	for _, t := range tenanters {
		nJobs += len(tenanter.RegionsForProviderAndTenant(provider, t))
	}
	glog.Infof("elb list start provider=%s account_filter=%q tenant_count=%d list_jobs=%d",
		provider.String(), scope.AccountName(ctx), len(tenanters), nJobs)

	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		all []*Instance
	)
	wg.Add(nJobs)
	for _, t := range tenanters {
		regions := tenanter.RegionsForProviderAndTenant(provider, t)
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
	before := len(all)
	all = scope.FilterSliceBySystemListTag(ctx, all, func(it *Instance) string {
		if it == nil {
			return ""
		}
		return it.SystemTagsDisplay
	})
	if before != len(all) {
		glog.Infof("elb list system_tag filter provider=%s account_filter=%q before=%d after=%d",
			provider.String(), scope.AccountName(ctx), before, len(all))
	}
	glog.Infof("elb list done provider=%s total=%d elapsed=%v",
		provider.String(), len(all), time.Since(begin))
	return all, nil
}

func listHuaweiElbByRegion(tenant tenanter.Tenanter, region tenanter.Region) ([]*Instance, error) {
	begin := time.Now()
	t, ok := tenant.(*tenanter.AccessKeyTenant)
	if !ok {
		return nil, nil
	}
	rName := region.GetName()
	baseAuth := basic.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).Build()
	iamHc := hwiam.IamClientBuilder().
		WithRegion(huaweicloudregion.EndpointForService("iam", rName)).
		WithCredential(baseAuth).
		WithHttpConfig(huaweicloudregion.SDKHttpConfig()).
		Build()
	iamCli := hwiam.NewIamClient(iamHc)
	projResp, err := huaweicloudregion.KeystoneListProjectsResolveProject(iamCli, rName)
	if err != nil || projResp == nil || projResp.Projects == nil || len(*projResp.Projects) == 0 {
		if err == nil {
			err = errors.New("empty project list")
		}
		return nil, errors.Wrapf(err, "Huawei KeystoneListProjects regionName %s", rName)
	}
	projectID := (*projResp.Projects)[0].Id

	auth := basic.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).WithProjectId(projectID).Build()
	elbCli := hwelb.NewElbClient(hwelb.ElbClientBuilder().
		WithRegion(huaweicloudregion.EndpointForService("elb", rName)).
		WithCredential(auth).
		WithHttpConfig(huaweicloudregion.SDKHttpConfig()).
		Build())
	elbV2Cli := hwelbv2.NewElbClient(hwelbv2.ElbClientBuilder().
		WithRegion(huaweicloudregion.EndpointForService("elb", rName)).
		WithCredential(auth).
		WithHttpConfig(huaweicloudregion.SDKHttpConfig()).
		Build())
	eipCli := hweip.NewEipClient(hweip.EipClientBuilder().
		// 华为云 EIP(v2) 走 vpc.<region>.myhuaweicloud.com
		WithRegion(huaweicloudregion.EndpointForService("vpc", rName)).
		WithCredential(auth).
		WithHttpConfig(huaweicloudregion.SDKHttpConfig()).
		Build())

	eipBandwidth := make(map[string]int32)
	eipReq := new(eipmodel.ListPublicipsRequest)
	eipLimit := int32(2000)
	eipReq.Limit = &eipLimit
	var eipCount int
	eipResp, e := huaweicloudregion.DoWithTransientNetworkRetry(func() (*eipmodel.ListPublicipsResponse, error) {
		return eipCli.ListPublicips(eipReq)
	})
	if e == nil && eipResp != nil && eipResp.Publicips != nil {
		eipCount = len(*eipResp.Publicips)
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
	} else if e != nil {
		glog.Warningf("elb list eip preload failed account=%s region=%s err=%v", tenant.AccountName(), rName, e)
	}
	glog.Infof("elb list eip preload account=%s region=%s eips=%d lookup_keys=%d", tenant.AccountName(), rName, eipCount, len(eipBandwidth))

	var pending []struct {
		lb          elbmodel.LoadBalancer
		listeners   string
		publicIP    string
		bandwidth   int32
	}
	req := new(elbmodel.ListLoadBalancersRequest)
	limit := int32(200)
	req.Limit = &limit
	page := 0
	var lbCount, listenerErrCount int
	for {
		page++
		resp, err := huaweicloudregion.DoWithTransientNetworkRetry(func() (*elbmodel.ListLoadBalancersResponse, error) {
			return elbCli.ListLoadBalancers(req)
		})
		if err != nil {
			return nil, errors.Wrap(err, "Huawei ELB ListLoadBalancers error")
		}
		if resp != nil && resp.Loadbalancers != nil {
			lbCount += len(*resp.Loadbalancers)
			for _, lb := range *resp.Loadbalancers {
				publicIP, bandwidth := pickPublicIPAndBandwidth(lb, eipBandwidth)
				listeners, lerr := queryListeners(elbCli, lb.Id)
				if lerr != nil {
					listenerErrCount++
					glog.Warningf("elb list listeners failed account=%s region=%s lb_id=%s lb_name=%q err=%v",
						tenant.AccountName(), rName, lb.Id, lb.Name, lerr)
				}
				pending = append(pending, struct {
					lb          elbmodel.LoadBalancer
					listeners   string
					publicIP    string
					bandwidth   int32
				}{lb: lb, listeners: strings.Join(listeners, "，"), publicIP: publicIP, bandwidth: bandwidth})
			}
		}
		if resp == nil || resp.PageInfo == nil || resp.PageInfo.NextMarker == nil || strings.TrimSpace(*resp.PageInfo.NextMarker) == "" {
			break
		}
		req.Marker = resp.PageInfo.NextMarker
	}

	type elbTagFetch struct {
		pairs [][2]string
		err   error
	}
	tagFetches := make([]elbTagFetch, len(pending))
	var tagOK, tagFail int32
	glog.Infof("Huawei ELB ShowLoadbalancerTags pull begin account=%s region=%s lbs=%d",
		tenant.AccountName(), rName, len(pending))
	var tagWG sync.WaitGroup
	tsem := make(chan struct{}, 10)
	for i := range pending {
		lid := strings.TrimSpace(pending[i].lb.Id)
		if lid == "" {
			continue
		}
		i := i
		tagWG.Add(1)
		go func() {
			defer tagWG.Done()
			tsem <- struct{}{}
			defer func() { <-tsem }()
			tr, terr := huaweicloudregion.DoWithTransientNetworkRetry(func() (*elbv2model.ShowLoadbalancerTagsResponse, error) {
				return elbV2Cli.ShowLoadbalancerTags(&elbv2model.ShowLoadbalancerTagsRequest{LoadbalancerId: lid})
			})
			if terr != nil {
				atomic.AddInt32(&tagFail, 1)
				tagFetches[i].err = terr
				glog.Warningf("Huawei ELB ShowLoadbalancerTags failed loadbalancer_id=%s account=%s region=%s err=%v (需 elb:loadbalancer:get 或含查询 LB 标签的只读)",
					lid, tenant.AccountName(), rName, terr)
				return
			}
			atomic.AddInt32(&tagOK, 1)
			tagFetches[i].pairs = huaweitags.PairsFromELBShowLoadbalancerTags(tr.Tags)
		}()
	}
	tagWG.Wait()
	glog.Infof("Huawei ELB ShowLoadbalancerTags pull end account=%s region=%s ok=%d fail=%d",
		tenant.AccountName(), rName, tagOK, tagFail)

	out := make([]*Instance, 0, len(pending))
	for i := range pending {
		row := pending[i]
		listPairs := huaweitags.PairsFromELBTags(row.lb.Tags)
		var merged [][2]string
		if tagFetches[i].err == nil && len(tagFetches[i].pairs) > 0 {
			merged = huaweitags.MergePairsPreferPrimary(tagFetches[i].pairs, listPairs)
		} else {
			merged = listPairs
		}
		userPairs := huaweitags.FilterPairsExcludingHuaweiSysPrefix(merged)
		lbName := strings.TrimSpace(row.lb.Name)
		ev := envtags.EnvTagOrNameFallback(envtags.FromPairs(envtags.ECSKey(), merged), lbName)
		nvSem := envtags.NodeTagOrNameFallback(envtags.FromPairs(envtags.NodeTagKey(), merged), lbName)
		nodeDisp := envtags.FormatNodeTagDisplay(envtags.CloudTypeLabelZH(pbtenant.CloudProvider_huawei), rName, nvSem)
		out = append(out, &Instance{
			Provider:          "huawei",
			AccountName:       tenant.AccountName(),
			RegionName:        rName,
			ID:                row.lb.Id,
			Name:              row.lb.Name,
			InstanceType:      elbTypeText(row.lb),
			IPv4Private:       row.lb.VipAddress,
			IPv4Public:        row.publicIP,
			Listeners:         row.listeners,
			IPv4BandwidthMbit: row.bandwidth,
			OnlineTime:        row.lb.CreatedAt,
			Status:            fmt.Sprint(row.lb.OperatingStatus),
			VpcID:             row.lb.VpcId,
			SystemTagsDisplay: strings.TrimSpace(envtags.FromPairs(envtags.SystemTagKey(), merged)),
			UserTagsDisplay:   huaweitags.FormatPairsDisplay(userPairs),
			EnvTagValue:       ev,
			NodeTagValue:      nodeDisp,
		})
	}
	glog.Infof("elb list region done account=%s region=%s pages=%d lbs=%d out=%d listener_err=%d elapsed=%v",
		tenant.AccountName(), rName, page, lbCount, len(out), listenerErrCount, time.Since(begin))
	return out, nil
}

func queryListeners(cli *hwelb.ElbClient, lbID string) ([]string, error) {
	req := new(elbmodel.ListListenersRequest)
	req.Limit = int32Ptr(200)
	req.LoadbalancerId = &[]string{lbID}
	var out []string
	for {
		resp, err := huaweicloudregion.DoWithTransientNetworkRetry(func() (*elbmodel.ListListenersResponse, error) {
			return cli.ListListeners(req)
		})
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
	if len(lb.Publicips) > 0 {
		for _, p := range lb.Publicips {
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
