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
	EnterpriseProjectName string `json:"enterpriseProjectName"`
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
	glog.Infof("waf list start provider=%s account_filter=%q tenant_count=%d waf_jobs=%d",
		provider.String(), scope.AccountName(ctx), len(tenanters), nJobs)

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		all      []*Instance
		firstErr error
	)
	wg.Add(nJobs)
	for _, t := range tenanters {
		for _, wafReg := range huaweiWAFRegionsForTenant(t) {
			go func(tenant tenanter.Tenanter, wafRegion string) {
				defer wg.Done()
				items, err := listHuaweiWafForTenant(tenant, wafRegion)
				if err != nil {
					glog.Errorf("waf list failed account=%s waf_region=%s err=%v", tenant.AccountName(), wafRegion, err)
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					return
				}
				mu.Lock()
				all = append(all, items...)
				mu.Unlock()
			}(t, wafReg)
		}
	}
	wg.Wait()
	if len(all) == 0 && firstErr != nil {
		return nil, firstErr
	}

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
	return nil
}

// huaweiWAFRegionsForTenant 默认华东-上海一（cn-east-3）；可用 CLOUD_FITTER_HUAWEI_WAF_REGIONS（逗号分隔）覆盖。
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

	epsSpec, err := resolveWafEnterpriseProject(t)
	if err != nil {
		return nil, errors.Wrap(err, "resolve WAF enterprise_project_id")
	}
	glog.Infof("waf list account=%s region=%s iam_project=%s eps_query=%q eps_name=%q eps_from=%s",
		t.AccountName(), rName, projectID, epsSpec.QueryID, epsSpec.Name, epsSpec.ConfigKey)

	policyNames, err := loadWafPolicyNames(wafCli, epsSpec.QueryID)
	if err != nil {
		glog.Warningf("waf list policy names account=%s region=%s err=%v", t.AccountName(), rName, err)
		policyNames = map[string]string{}
	}
	cloudServers, err := loadCloudHostServers(wafCli, epsSpec.QueryID)
	if err != nil {
		glog.Warningf("waf list cloud host servers account=%s region=%s err=%v", t.AccountName(), rName, err)
		cloudServers = map[string]string{}
	}

	pageSize := int32(-1)
	req := &wafmodel.ListCompositeHostsRequest{
		EnterpriseProjectId: strPtr(epsSpec.QueryID),
		Pagesize:            &pageSize,
	}
	resp, err := wafCli.ListCompositeHosts(req)
	if err != nil {
		return nil, errors.Wrapf(err, "ListCompositeHosts region=%s enterprise_project_id=%s", rName, epsSpec.QueryID)
	}
	if resp == nil || resp.Items == nil {
		return nil, nil
	}

	certByHost, err := loadWafCertificateNames(wafCli, epsSpec.QueryID, resp.Items)
	if err != nil {
		glog.Warningf("waf list certificate names account=%s region=%s err=%v", t.AccountName(), rName, err)
		certByHost = map[string]string{}
	}

	var out []*Instance
	for i := range *resp.Items {
		item := &(*resp.Items)[i]
		inst := compositeHostToInstance(item, t.AccountName(), rName, epsSpec, policyNames, certByHost, cloudServers)
		out = append(out, inst)
	}
	return out, nil
}

func compositeHostToInstance(
	h *wafmodel.CompositeHostResponse,
	accountName, regionName string,
	eps enterpriseProjectSpec,
	policyNames, certByHost, cloudServers map[string]string,
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

	certName := lookupCertName(certByHost, h, hostname)

	origin := formatWafServers(h.Server)
	if origin == "" {
		if v := cloudServers[strVal(h.Id)]; v != "" {
			origin = v
		} else if v := cloudServers[strVal(h.Hostid)]; v != "" {
			origin = v
		} else if v := cloudServers[hostname]; v != "" {
			origin = v
		}
	}

	epsID := strVal(h.EnterpriseProjectId)
	if epsID == "" {
		epsID = eps.QueryID
	}
	epsName := eps.Name
	if epsName == "" {
		epsName = epsID
	}

	return &Instance{
		Provider:              pbtenant.CloudProvider_huawei.String(),
		AccountName:           accountName,
		RegionName:            regionName,
		EnterpriseProjectID:   epsID,
		EnterpriseProjectName: epsName,
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
		OriginServers:       origin,
		CreatedAt:           formatWafTimestamp(h.Timestamp),
		AccessCode:          strVal(h.AccessCode),
		WebTag:              strVal(h.WebTag),
		Description:         strVal(h.Description),
	}
}

func loadCloudHostServers(cli *hwwaf.WafClient, epsID string) (map[string]string, error) {
	out := make(map[string]string)
	pageSize := int32(-1)
	req := &wafmodel.ListHostRequest{
		EnterpriseProjectId: strPtr(epsID),
		Pagesize:            &pageSize,
	}
	resp, err := cli.ListHost(req)
	if err != nil {
		return out, err
	}
	if resp == nil || resp.Items == nil {
		return out, nil
	}
	for i := range *resp.Items {
		item := &(*resp.Items)[i]
		servers := formatCloudWafServers(item.Server)
		if servers == "" {
			continue
		}
		if id := strVal(item.Id); id != "" {
			out[id] = servers
		}
		if id := strVal(item.Hostid); id != "" {
			out[id] = servers
		}
		if hn := strVal(item.Hostname); hn != "" {
			out[hn] = servers
		}
	}
	return out, nil
}

func formatCloudWafServers(servers *[]wafmodel.CloudWafServer) string {
	if servers == nil || len(*servers) == 0 {
		return ""
	}
	var parts []string
	for i := range *servers {
		s := &(*servers)[i]
		addr := strings.TrimSpace(s.Address)
		if addr == "" {
			continue
		}
		if s.Port > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", addr, s.Port))
		} else {
			parts = append(parts, addr)
		}
	}
	return strings.Join(parts, ", ")
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

func loadWafCertificateNames(cli *hwwaf.WafClient, epsID string, items *[]wafmodel.CompositeHostResponse) (map[string]string, error) {
	out, err := loadWafCertNamesFromListAPI(cli, epsID)
	if err != nil {
		out = map[string]string{}
	}
	enrichWafCertNamesFromShowHost(cli, epsID, items, out)
	return out, nil
}

func loadWafCertNamesFromListAPI(cli *hwwaf.WafClient, epsID string) (map[string]string, error) {
	out := make(map[string]string)
	const pageSize int32 = 100
	page := int32(1)
	for {
		req := &wafmodel.ListCertificatesRequest{
			EnterpriseProjectId: strPtr(epsID),
			Page:                &page,
			Pagesize:            int32Ptr(pageSize),
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
			if strings.TrimSpace(c.Name) == "" {
				continue
			}
			if c.BindHost == nil {
				continue
			}
			for j := range *c.BindHost {
				bh := &(*c.BindHost)[j]
				if bh.Id != nil {
					out[strings.TrimSpace(*bh.Id)] = c.Name
				}
				if bh.Hostname != nil {
					hn := strings.TrimSpace(*bh.Hostname)
					if hn != "" {
						out[hn] = c.Name
						out[strings.ToLower(hn)] = c.Name
						out[wafHostnameBase(hn)] = c.Name
					}
				}
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

func enrichWafCertNamesFromShowHost(cli *hwwaf.WafClient, epsID string, items *[]wafmodel.CompositeHostResponse, out map[string]string) {
	if items == nil || out == nil {
		return
	}
	const workers = 8
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i := range *items {
		item := &(*items)[i]
		hostID := strVal(item.Id)
		if hostID == "" {
			hostID = strVal(item.Hostid)
		}
		hostname := strVal(item.Hostname)
		if hostID == "" {
			continue
		}
		if lookupCertName(out, item, hostname) != "" {
			continue
		}
		wafType := strings.ToLower(strings.TrimSpace(strVal(item.WafType)))

		wg.Add(1)
		go func(hostID, hostname, wafType string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			certName := fetchWafHostCertificateName(cli, epsID, hostID, wafType)
			if certName == "" {
				return
			}
			mu.Lock()
			out[hostID] = certName
			if hostname != "" {
				out[hostname] = certName
				out[strings.ToLower(hostname)] = certName
				out[wafHostnameBase(hostname)] = certName
			}
			mu.Unlock()
		}(hostID, hostname, wafType)
	}
	wg.Wait()
}

func fetchWafHostCertificateName(cli *hwwaf.WafClient, epsID, hostID, wafType string) string {
	if wafType == "premium" {
		resp, err := cli.ShowPremiumHost(&wafmodel.ShowPremiumHostRequest{
			EnterpriseProjectId: strPtr(epsID),
			HostId:              hostID,
		})
		if err != nil || resp == nil {
			return ""
		}
		return strVal(resp.Certificatename)
	}
	resp, err := cli.ShowHost(&wafmodel.ShowHostRequest{
		EnterpriseProjectId: strPtr(epsID),
		InstanceId:          hostID,
	})
	if err != nil || resp == nil {
		return ""
	}
	return strVal(resp.Certificatename)
}

func lookupCertName(certByHost map[string]string, h *wafmodel.CompositeHostResponse, hostname string) string {
	if certByHost == nil {
		return ""
	}
	for _, k := range []string{
		hostname,
		strings.ToLower(hostname),
		wafHostnameBase(hostname),
		strVal(h.Id),
		strVal(h.Hostid),
	} {
		if k == "" {
			continue
		}
		if v := certByHost[k]; v != "" {
			return v
		}
	}
	return ""
}

func wafHostnameBase(hostname string) string {
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return ""
	}
	i := strings.LastIndex(hostname, ":")
	if i <= 0 {
		return hostname
	}
	hostPart := hostname[:i]
	portPart := hostname[i+1:]
	if !strings.Contains(hostPart, ".") {
		return hostname
	}
	if _, err := strconv.Atoi(portPart); err != nil {
		return hostname
	}
	return hostPart
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
