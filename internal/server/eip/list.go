package eip

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
	EipId             string `json:"eipId"`
	Eip               string `json:"eip"`
	// EipName 弹性公网名称：华为 ListPublicips.alias，空则退回 bandwidth_name（与控制台「名称」一致）
	EipName           string `json:"eipName"`
	BandwidthType     string `json:"bandwidthType"`
	BandwidthSizeMbit int32  `json:"bandwidthSizeMbit"`
	BindInstanceType  string `json:"bindInstanceType"`
	BindInstanceName  string `json:"bindInstanceName"`
	BindInstanceId    string `json:"bindInstanceId"`
	PrivateIpAddress  string `json:"privateIpAddress"`
	OnlineTime        string `json:"onlineTime"`
	Status            string `json:"status"`
	EnterpriseProject string `json:"enterpriseProjectId"`
	// 系统标签(展示)：用户自定义「系统」标签键的值（CLOUD_FITTER_SYSTEM_TAG_KEY / 默认 system），来自 ShowPublicipTags
	SystemTagsDisplay string `json:"systemTagsDisplay"`
	// 用户自定义标签：ShowPublicipTags（key=value; 拼接）
	UserTagsDisplay string `json:"userTagsDisplay"`
	// 环境(标签)：与 ECS/RDS 相同键名合并标签后 + 名字规则（alias/带宽名 等作名字线索）
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
	glog.Infof("eip list start provider=%s account_filter=%q tenant_count=%d list_jobs=%d",
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
	eipHc := hweip.EipClientBuilder().
		// 华为云 EIP(v2) 走 vpc.<region>.myhuaweicloud.com
		WithRegion(huaweicloudregion.EndpointForService("vpc", rName)).
		WithCredential(auth).
		WithHttpConfig(huaweicloudregion.SDKHttpConfig()).
		Build()
	eipCli := hweip.NewEipClient(eipHc)
	req := new(eipmodel.ListPublicipsRequest)
	limit := int32(2000)
	req.Limit = &limit
	resp, err := huaweicloudregion.DoWithTransientNetworkRetry(func() (*eipmodel.ListPublicipsResponse, error) {
		return eipCli.ListPublicips(req)
	})
	if err != nil {
		return nil, errors.Wrap(err, "Huawei EIP ListPublicips error")
	}
	if resp == nil || resp.Publicips == nil {
		glog.Infof("eip list region done account=%s region=%s rows=0 elapsed=%v",
			tenant.AccountName(), rName, time.Since(begin))
		return nil, nil
	}
	pubList := *resp.Publicips
	out := make([]*Instance, 0, len(pubList))
	nameHints := make([]string, len(pubList))
	for i, it := range pubList {
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
		hint := strings.TrimSpace(deref(it.Alias))
		if hint == "" {
			hint = strings.TrimSpace(deref(it.BandwidthName))
		}
		row.EipName = hint
		nameHints[i] = hint
		out = append(out, row)
	}
	// 华为云官方「查询弹性公网IP的标签」ShowPublicipTags
	var tagWG sync.WaitGroup
	tsem := make(chan struct{}, 10)
	var tagFail int32
	for i := range out {
		i := i
		pid := strings.TrimSpace(out[i].EipId)
		if pid == "" {
			continue
		}
		tagWG.Add(1)
		go func() {
			defer tagWG.Done()
			tsem <- struct{}{}
			defer func() { <-tsem }()
			tw, terr := huaweicloudregion.DoWithTransientNetworkRetry(func() (*eipmodel.ShowPublicipTagsResponse, error) {
				return eipCli.ShowPublicipTags(&eipmodel.ShowPublicipTagsRequest{PublicipId: pid})
			})
			if terr != nil {
				atomic.AddInt32(&tagFail, 1)
				glog.Warningf("Huawei EIP ShowPublicipTags failed publicip_id=%s account=%s region=%s err=%v (需 vpc:publicIp:get 或含查询 EIP 标签的只读)",
					pid, tenant.AccountName(), rName, terr)
				nameHint := ""
				if i < len(nameHints) {
					nameHint = nameHints[i]
				}
				out[i].EnvTagValue = envtags.EnvTagOrNameFallback("", nameHint)
				nvSem := envtags.NodeTagOrNameFallback("", nameHint)
				out[i].NodeTagValue = envtags.FormatNodeTagDisplay(envtags.CloudTypeLabelZH(pbtenant.CloudProvider_huawei), rName, nvSem)
				return
			}
			pairs := huaweitags.PairsFromEIPShowPublicipTags(tw.Tags)
			userPairs := huaweitags.FilterPairsExcludingHuaweiSysPrefix(pairs)
			out[i].UserTagsDisplay = huaweitags.FormatPairsDisplay(userPairs)
			out[i].SystemTagsDisplay = strings.TrimSpace(envtags.FromPairs(envtags.SystemTagKey(), pairs))
			envK, nodeK := envtags.ECSKey(), envtags.NodeTagKey()
			nameHint := ""
			if i < len(nameHints) {
				nameHint = nameHints[i]
			}
			ev := envtags.EnvTagOrNameFallback(envtags.FromPairs(envK, pairs), nameHint)
			nvSem := envtags.NodeTagOrNameFallback(envtags.FromPairs(nodeK, pairs), nameHint)
			out[i].EnvTagValue = ev
			out[i].NodeTagValue = envtags.FormatNodeTagDisplay(envtags.CloudTypeLabelZH(pbtenant.CloudProvider_huawei), rName, nvSem)
		}()
	}
	tagWG.Wait()
	if tagFail > 0 {
		glog.Infof("Huawei EIP ShowPublicipTags partial failures account=%s region=%s fail=%d",
			tenant.AccountName(), rName, tagFail)
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
