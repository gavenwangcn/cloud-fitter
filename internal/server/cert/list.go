package cert

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hwiam "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3"
	iammodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3/model"
	hwscm "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/scm/v3"
	scmmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/scm/v3/model"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweicloudregion"
	"github.com/cloud-fitter/cloud-fitter/internal/server/scope"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

// Instance 华为云 CCM（SCM API）证书列表行，供 JSON API 返回。
type Instance struct {
	Provider              string `json:"provider"`
	AccountName           string `json:"accountName"`
	RegionName            string `json:"regionName"`
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Domain                string `json:"domain"`
	Sans                  string `json:"sans"`
	SignatureAlgorithm    string `json:"signatureAlgorithm"`
	DeploySupport         bool   `json:"deploySupport"`
	CertificateType       string `json:"certificateType"`
	Brand                 string `json:"brand"`
	ExpireTime            string `json:"expireTime"`
	DomainType            string `json:"domainType"`
	ValidityPeriodMonths  int32  `json:"validityPeriodMonths"`
	Status                string `json:"status"`
	DomainCount           int32  `json:"domainCount"`
	WildcardCount         int32  `json:"wildcardCount"`
	Description           string `json:"description"`
	EnterpriseProjectID   string `json:"enterpriseProjectId"`
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

	scmRegions := huaweiSCMEndpointRegions()
	glog.Infof("cert list start provider=%s account_filter=%q tenant_count=%d scm_endpoint_regions=%v",
		provider.String(), scope.AccountName(ctx), len(tenanters), scmRegions)

	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		all []*Instance
	)
	wg.Add(len(tenanters) * len(scmRegions))
	for _, t := range tenanters {
		for _, scmReg := range scmRegions {
			go func(tenant tenanter.Tenanter, scmRegion string) {
				defer wg.Done()
				items, err := listHuaweiCertificatesForTenant(tenant, scmRegion)
				if err != nil {
					glog.Errorf("cert list failed account=%s scm_region=%s err=%v", tenant.AccountName(), scmRegion, err)
					return
				}
				mu.Lock()
				all = append(all, items...)
				mu.Unlock()
			}(t, scmReg)
		}
	}
	wg.Wait()
	// 多 SCM 接入区可能返回相同证书；同一账号下证书 ID 全局唯一，按账号+ID 去重。
	seen := make(map[string]struct{}, len(all))
	uniq := make([]*Instance, 0, len(all))
	for _, x := range all {
		if x == nil {
			continue
		}
		k := fmt.Sprintf("%s|%s", x.AccountName, x.ID)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		uniq = append(uniq, x)
	}
	glog.Infof("cert list done provider=%s rows_raw=%d rows_dedup=%d elapsed=%v",
		provider.String(), len(all), len(uniq), time.Since(begin))
	return uniq, nil
}

// huaweiSCMEndpointRegions 返回要轮询的 SCM（CCM）接入 Region 列表（每个对应 scm.<region>.myhuaweicloud.com）。
// 并非所有业务 Region 都有 SCM 域名；与 ECS 枚举地域无关，需单独配置接入区。
//
// 优先级：
//  1. CLOUD_FITTER_HUAWEI_SCM_REGIONS — 逗号分隔，例如 "cn-north-4,ap-southeast-1,ru-moscow-1"
//  2. CLOUD_FITTER_HUAWEI_SCM_REGION — 兼容旧版，仅单个接入区
//  3. 默认 cn-north-4、ap-southeast-1、ru-moscow-1（华北 SCM + 香港/国际站 + 俄罗斯莫斯科接入）
func huaweiSCMEndpointRegions() []string {
	if raw := strings.TrimSpace(os.Getenv("CLOUD_FITTER_HUAWEI_SCM_REGIONS")); raw != "" {
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
	if single := strings.TrimSpace(os.Getenv("CLOUD_FITTER_HUAWEI_SCM_REGION")); single != "" {
		return []string{single}
	}
	return []string{"cn-north-4", "ap-southeast-1", "ru-moscow-1"}
}

func listHuaweiCertificatesForTenant(tenant tenanter.Tenanter, endpointRegion string) ([]*Instance, error) {
	begin := time.Now()
	t, ok := tenant.(*tenanter.AccessKeyTenant)
	if !ok {
		return nil, nil
	}
	rName := endpointRegion
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
	scmCli := hwscm.NewScmClient(hwscm.ScmClientBuilder().
		WithRegion(huaweicloudregion.EndpointForService("scm", rName)).
		WithCredential(auth).
		Build())

	const pageSize int32 = 50
	var offset int32
	var out []*Instance
	var page int
	for {
		page++
		req := new(scmmodel.ListCertificatesRequest)
		limit := pageSize
		req.Limit = &limit
		req.Offset = &offset

		resp, err := scmCli.ListCertificates(req)
		if err != nil {
			return nil, errors.Wrap(err, "Huawei SCM ListCertificates error")
		}
		n := 0
		if resp != nil && resp.Certificates != nil {
			n = len(*resp.Certificates)
			for _, c := range *resp.Certificates {
				out = append(out, &Instance{
					Provider:             "huawei",
					AccountName:          tenant.AccountName(),
					RegionName:           rName, // SCM 接入地域（证书为账号级，非资源所属 VPC 地域）
					ID:                   c.Id,
					Name:                 c.Name,
					Domain:               c.Domain,
					Sans:                 c.Sans,
					SignatureAlgorithm:   c.SignatureAlgorithm,
					DeploySupport:        c.DeploySupport,
					CertificateType:      c.Type,
					Brand:                c.Brand,
					ExpireTime:           c.ExpireTime,
					DomainType:           c.DomainType,
					ValidityPeriodMonths: c.ValidityPeriod,
					Status:               c.Status,
					DomainCount:          c.DomainCount,
					WildcardCount:        c.WildcardCount,
					Description:          c.Description,
					EnterpriseProjectID:  c.EnterpriseProjectId,
				})
			}
		}
		var total int32
		if resp != nil && resp.TotalCount != nil {
			total = *resp.TotalCount
		}
		glog.Infof("cert list page account=%s region=%s page=%d offset=%d batch=%d total=%d elapsed=%v",
			tenant.AccountName(), rName, page, offset, n, total, time.Since(begin))
		if n == 0 {
			break
		}
		offset += pageSize
		if total > 0 && offset >= total {
			break
		}
		if n < int(pageSize) {
			break
		}
	}
	return out, nil
}
