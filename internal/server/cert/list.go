package cert

import (
	"context"
	"fmt"
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

	regions := tenanter.GetAllRegionIds(provider)
	glog.Infof("cert list start provider=%s account_filter=%q tenant_count=%d region_count=%d",
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
				items, err := listHuaweiCertificatesByRegion(tenant, region)
				if err != nil {
					glog.Errorf("cert list failed account=%s region=%s err=%v", tenant.AccountName(), region.GetName(), err)
					return
				}
				mu.Lock()
				all = append(all, items...)
				mu.Unlock()
			}(t, r)
		}
	}
	wg.Wait()
	// 同一账号下证书 ID 全局唯一；多地域 endpoint 可能返回相同条目，按账号+ID 去重保留首次出现的地域。
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

func listHuaweiCertificatesByRegion(tenant tenanter.Tenanter, region tenanter.Region) ([]*Instance, error) {
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
					RegionName:           rName,
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
