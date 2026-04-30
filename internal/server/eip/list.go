package eip

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hweip "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/eip/v2"
	eipmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/eip/v2/model"
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
	EipId             string `json:"eipId"`
	Eip               string `json:"eip"`
	BandwidthType     string `json:"bandwidthType"`
	BandwidthSizeMbit int32  `json:"bandwidthSizeMbit"`
	BindInstanceType  string `json:"bindInstanceType"`
	BindInstanceName  string `json:"bindInstanceName"`
	BindInstanceId    string `json:"bindInstanceId"`
	PrivateIpAddress  string `json:"privateIpAddress"`
	OnlineTime        string `json:"onlineTime"`
	Status            string `json:"status"`
	EnterpriseProject string `json:"enterpriseProjectId"`
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

	regions := tenanter.GetAllRegionIds(provider)
	glog.Infof("eip list start provider=%s account_filter=%q tenant_count=%d region_count=%d",
		provider.String(), scope.AccountName(ctx), len(tenanters), len(regions))
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
				items, err := listHuaweiEipByRegion(tenant, region)
				if err != nil {
					glog.Errorf("eip list failed account=%s region=%s err=%v", tenant.AccountName(), region.GetName(), err)
					return
				}
				mu.Lock()
				all = append(all, items...)
				mu.Unlock()
			}(t, r)
		}
	}
	wg.Wait()
	glog.Infof("eip list done provider=%s total=%d elapsed=%v",
		provider.String(), len(all), time.Since(begin))
	return all, nil
}

func listHuaweiEipByRegion(tenant tenanter.Tenanter, region tenanter.Region) ([]*Instance, error) {
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
	eipHc := hweip.EipClientBuilder().
		// 华为云 EIP(v2) 走 vpc.<region>.myhuaweicloud.com
		WithRegion(huaweicloudregion.EndpointForService("vpc", rName)).
		WithCredential(auth).
		Build()
	eipCli := hweip.NewEipClient(eipHc)
	req := new(eipmodel.ListPublicipsRequest)
	limit := int32(2000)
	req.Limit = &limit
	resp, err := eipCli.ListPublicips(req)
	if err != nil {
		return nil, errors.Wrap(err, "Huawei EIP ListPublicips error")
	}
	if resp == nil || resp.Publicips == nil {
		glog.Infof("eip list region done account=%s region=%s rows=0 elapsed=%v",
			tenant.AccountName(), rName, time.Since(begin))
		return nil, nil
	}
	out := make([]*Instance, 0, len(*resp.Publicips))
	for _, it := range *resp.Publicips {
		row := &Instance{
			Provider:          "huawei",
			AccountName:       tenant.AccountName(),
			RegionName:        rName,
			EipId:             deref(it.Id),
			Eip:               deref(it.PublicIpAddress),
			BandwidthType:     enumText(it.BandwidthShareType),
			BandwidthSizeMbit: derefI32(it.BandwidthSize),
			BindInstanceType:  inferBindType(it.PortId),
			BindInstanceName:  "",
			BindInstanceId:    deref(it.PortId),
			PrivateIpAddress:  deref(it.PrivateIpAddress),
			OnlineTime:        timeText(it.CreateTime),
			Status:            enumText(it.Status),
			EnterpriseProject: deref(it.EnterpriseProjectId),
		}
		out = append(out, row)
	}
	glog.Infof("eip list region done account=%s region=%s rows=%d elapsed=%v",
		tenant.AccountName(), rName, len(out), time.Since(begin))
	return out, nil
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

func enumText(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func timeText(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func inferBindType(portID *string) string {
	if portID == nil || *portID == "" {
		return "未绑定"
	}
	return "端口绑定"
}
