package waf

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hwiam "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3"
	hwwaf "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/waf/v1"
	wafmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/waf/v1/model"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweicloudregion"
	"github.com/cloud-fitter/cloud-fitter/internal/server/scope"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

// Instance 华为云 WAF 防护域名行，字段与控制台「网站设置」列表对齐。
type Instance struct {
	Provider              string `json:"provider"`
	AccountName           string `json:"accountName"`
	RegionName            string `json:"regionName"`
	EnterpriseProjectID   string `json:"enterpriseProjectId"`
	ID                    string `json:"id"`
	HostID                string `json:"hostId"`
	Hostname              string `json:"hostname"`
	AccessMode            string `json:"accessMode"`
	ResourceID            string `json:"resourceId"`
	AccessStatus          string `json:"accessStatus"`
	AccessStatusCode      int32  `json:"accessStatusCode"`
	ProtectStatus         string `json:"protectStatus"`
	ProtectStatusCode     int32  `json:"protectStatusCode"`
	CertificateName       string `json:"certificateName"`
	PolicyID              string `json:"policyId"`
	PolicyName            string `json:"policyName"`
	OriginServers         string `json:"originServers"`
	CreatedAt             string `json:"createdAt"`
	AccessCode            string `json:"accessCode"`
	WebTag                string `json:"webTag"`
	Description           string `json:"description"`
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
		nJobs += len(huaweiWAFRegionsForTenant(t))
	}
	glog.Infof("waf list start provider=%s account_filter=%q tenant_count=%d waf_jobs=%d eps=%q",
		provider.String(), scope.AccountName(ctx), len(tenanters), nJobs, wafEnterpriseProjectID())

	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		all []*Instance
	)
	wg.Add(nJobs)
	for _, t := range tenanters {
		for _, wafReg := range huaweiWAFRegionsForTenant(t) {
			go func(tenant tenanter.Tenanter, wafRegion string) {
				defer wg.Done()
				items, err := listHuaweiWafForTenant(tenant, wafRegion)
				if err != nil {
					glog.Errorf("waf list failed account=%s waf_region=%s err=%v", tenant.AccountName(), wafRegion, err)
					return
				}
				mu.Lock()
				all = append(all, items...)
				mu.Unlock()
			}(t, wafReg)
		}
	}
	wg.Wait()

	seen := make(map[string]struct{}, len(all))
	uniq := make([]*Instance, 0, len(all))
	for _, x := range all {
		if x == nil {
			continue
		}
		k := fmt.Sprintf("%s|%s|%s", x.AccountName, x.RegionName, x.ID)
		if x.ID == "" {
			k = fmt.Sprintf("%s|%s|%s", x.AccountName, x.RegionName, x.Hostname)
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		uniq = append(uniq, x)
	}
	glog.Infof("waf list done provider=%s rows_raw=%d rows_dedup=%d elapsed=%v",
		provider.String(), len(all), len(uniq), time.Since(begin))
	return uniq, nil
}

func wafEnterpriseProjectID() string {
	if v := strings.TrimSpace(os.Getenv("CLOUD_FITTER_HUAWEI_WAF_ENTERPRISE_PROJECT_ID")); v != "" {
		return v
	}
	return "dft_lc_infra"
}

func wafRegionsFromEnvOverride() []string {
	if raw := strings.TrimSpace(os.Getenv("CLOUD_FITTER_HUAWEI_WAF_REGIONS")); raw != "" {
		var out []string
		seen := make(map[string]struct{})
		for _, p := range strings.Split(raw, ",") {
			r := strings.TrimSpace(p)
			if r == "" {
				continue
			}
			if _, ok := seen[r]; ok {
				continue
			}
			seen[r] = struct{}{}
			out = append(out, r)
		}
		if len(out) > 0 {
			return out
		}
	}
	if single := strings.TrimSpace(os.Getenv("CLOUD_FITTER_HUAWEI_WAF_REGION")); single != "" {
		return []string{single}
	}
	return nil
}

// huaweiWAFRegionsForTenant 默认华东-上海一（cn-east-3）；可用环境变量覆盖。
func huaweiWAFRegionsForTenant(tenant tenanter.Tenanter) []string {
	if o := wafRegionsFromEnvOverride(); o != nil {
		return o
	}
	_ = tenant
	return []string{"cn-east-3"}
}

func listHuaweiWafForTenant(tenant tenanter.Tenanter, endpointRegion string) ([]*Instance, error) {
	t, ok := tenant.(*tenanter.AccessKeyTenant)
	if !ok {
		return nil, nil
	}
	rName := endpointRegion
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
	wafCli := hwwaf.NewWafClient(hwwaf.WafClientBuilder().
		WithRegion(huaweicloudregion.EndpointForService("waf", rName)).
		WithCredential(auth).
		WithHttpConfig(huaweicloudregion.SDKHttpConfig()).
		Build())

	epsID := wafEnterpriseProjectID()
	policyNames, err := loadWafPolicyNames(wafCli, epsID)
	if err != nil {
		glog.Warningf("waf list policy names account=%s region=%s err=%v", t.AccountName(), rName, err)
		policyNames = map[string]string{}
	}
	certByHost, err := loadWafCertNamesByHostname(wafCli, epsID)
	if err != nil {
		glog.Warningf("waf list certificate names account=%s region=%s err=%v", t.AccountName(), rName, err)
		certByHost = map[string]string{}
	}

	pageSize := int32(-1)
	req := &wafmodel.ListCompositeHostsRequest{
		EnterpriseProjectId: strPtr(epsID),
		Pagesize:            &pageSize,
	}
	resp, err := wafCli.ListCompositeHosts(req)
	if err != nil {
		return nil, errors.Wrapf(err, "ListCompositeHosts region=%s eps=%s", rName, epsID)
	}
	if resp == nil || resp.Items == nil {
		return nil, nil
	}

	var out []*Instance
	for i := range *resp.Items {
		item := &(*resp.Items)[i]
		inst := compositeHostToInstance(item, t.AccountName(), rName, epsID, policyNames, certByHost)
		out = append(out, inst)
	}
	return out, nil
}

func compositeHostToInstance(
	h *wafmodel.CompositeHostResponse,
	accountName, regionName, epsID string,
	policyNames, certByHost map[string]string,
) *Instance {
	hostname := strVal(h.Hostname)
	policyID := strVal(h.Policyid)
	resourceID := strVal(h.Id)
	if resourceID == "" {
		resourceID = strVal(h.Hostid)
	}
	wafType := strings.ToLower(strings.TrimSpace(strVal(h.WafType)))
	accessMode := wafTypeLabel(wafType)
	if regionName != "" {
		accessMode = fmt.Sprintf("%s-%s", accessMode, strings.ToUpper(regionName))
	}

	protectCode := int32(0)
	if h.ProtectStatus != nil {
		protectCode = *h.ProtectStatus
	}
	accessCode := int32(0)
	if h.AccessStatus != nil {
		accessCode = *h.AccessStatus
	}

	certName := certByHost[hostname]
	if certName == "" {
		certName = certByHost[strings.ToLower(hostname)]
	}

	return &Instance{
		Provider:            pbtenant.CloudProvider_huawei.String(),
		AccountName:         accountName,
		RegionName:          regionName,
		EnterpriseProjectID: epsID,
		ID:                  strVal(h.Id),
		HostID:              strVal(h.Hostid),
		Hostname:            hostname,
		AccessMode:          accessMode,
		ResourceID:          resourceID,
		AccessStatus:        accessStatusLabel(accessCode),
		AccessStatusCode:    accessCode,
		ProtectStatus:       protectStatusLabel(protectCode),
		ProtectStatusCode:   protectCode,
		CertificateName:     certName,
		PolicyID:            policyID,
		PolicyName:          policyNames[policyID],
		OriginServers:       formatWafServers(h.Server),
		CreatedAt:           formatWafTimestamp(h.Timestamp),
		AccessCode:          strVal(h.AccessCode),
		WebTag:              strVal(h.WebTag),
		Description:         strVal(h.Description),
	}
}

func loadWafPolicyNames(cli *hwwaf.WafClient, epsID string) (map[string]string, error) {
	out := make(map[string]string)
	const pageSize int32 = 100
	page := int32(1)
	for {
		req := &wafmodel.ListPolicyRequest{
			EnterpriseProjectId: strPtr(epsID),
			Page:                &page,
			Pagesize:            int32Ptr(pageSize),
		}
		resp, err := cli.ListPolicy(req)
		if err != nil {
			return out, err
		}
		if resp == nil || resp.Items == nil {
			break
		}
		for i := range *resp.Items {
			p := &(*resp.Items)[i]
			id := strVal(p.Id)
			if id != "" {
				out[id] = strVal(p.Name)
			}
		}
		if resp.Total == nil || int32(len(*resp.Items)) < pageSize || int32(len(out)) >= *resp.Total {
			break
		}
		page++
	}
	return out, nil
}

func loadWafCertNamesByHostname(cli *hwwaf.WafClient, epsID string) (map[string]string, error) {
	out := make(map[string]string)
	hostTrue := true
	const pageSize int32 = 100
	page := int32(1)
	for {
		req := &wafmodel.ListCertificatesRequest{
			EnterpriseProjectId: strPtr(epsID),
			Page:                &page,
			Pagesize:            int32Ptr(pageSize),
			Host:                &hostTrue,
		}
		resp, err := cli.ListCertificates(req)
		if err != nil {
			return out, err
		}
		if resp == nil || resp.Items == nil {
			break
		}
		for i := range *resp.Items {
			c := &(*resp.Items)[i]
			if c.BindHost == nil {
				continue
			}
			for j := range *c.BindHost {
				bh := &(*c.BindHost)[j]
				if bh.Hostname == nil {
					continue
				}
				hn := strings.TrimSpace(*bh.Hostname)
				if hn == "" {
					continue
				}
				out[hn] = c.Name
			}
		}
		if resp.Total == nil || int32(len(*resp.Items)) < pageSize {
			break
		}
		if int32(len(*resp.Items))*page >= *resp.Total {
			break
		}
		page++
	}
	return out, nil
}

func formatWafServers(servers *[]wafmodel.WafServer) string {
	if servers == nil || len(*servers) == 0 {
		return ""
	}
	var parts []string
	for i := range *servers {
		s := &(*servers)[i]
		addr := strVal(s.Address)
		if addr == "" {
			continue
		}
		if s.Port != nil && *s.Port > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", addr, *s.Port))
		} else {
			parts = append(parts, addr)
		}
	}
	return strings.Join(parts, ", ")
}

func formatWafTimestamp(ts *int64) string {
	if ts == nil || *ts == 0 {
		return ""
	}
	v := *ts
	// 毫秒时间戳
	if v > 1_000_000_000_000 {
		v /= 1000
	}
	return time.Unix(v, 0).In(time.FixedZone("CST", 8*3600)).Format("2006/01/02 15:04:05")
}

func wafTypeLabel(wafType string) string {
	switch wafType {
	case "cloud":
		return "云模式"
	case "premium":
		return "独享模式"
	default:
		if wafType == "" {
			return "未知模式"
		}
		return wafType
	}
}

func accessStatusLabel(code int32) string {
	switch code {
	case 0:
		return "未接入"
	case 1:
		return "已接入"
	default:
		return "未知(" + strconv.FormatInt(int64(code), 10) + ")"
	}
}

func protectStatusLabel(code int32) string {
	switch code {
	case -1:
		return "Bypass"
	case 0:
		return "暂停防护"
	case 1:
		return "防护中"
	default:
		return "未知(" + strconv.FormatInt(int64(code), 10) + ")"
	}
}

func strVal(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func int32Ptr(v int32) *int32 {
	return &v
}
