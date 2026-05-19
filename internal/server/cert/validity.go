package cert

import (
	"context"
	"strings"

	"github.com/golang/glog"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hwiam "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3"
	hwscm "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/scm/v3"
	scmmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/scm/v3/model"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweicloudregion"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

// EnrichValidityFromShow 调用 ShowCertificate 补全生效/失效时间（not_before / not_after），供 CMDB valid_from / valid_to。
func EnrichValidityFromShow(ctx context.Context, inst *Instance) error {
	_ = ctx
	if inst == nil || strings.TrimSpace(inst.ID) == "" {
		return nil
	}
	acc := strings.TrimSpace(inst.AccountName)
	if acc == "" {
		return errors.New("certificate instance missing accountName")
	}
	tenants, err := tenanter.GetTenanters(pbtenant.CloudProvider_huawei)
	if err != nil {
		return err
	}
	var tenant tenanter.Tenanter
	for _, t := range tenants {
		if t.AccountName() == acc {
			tenant = t
			break
		}
	}
	if tenant == nil {
		return errors.Errorf("no tenant for account %q", acc)
	}
	rName := strings.TrimSpace(inst.RegionName)
	if rName == "" {
		rName = "cn-north-4"
	}
	scmCli, err := newHuaweiScmClient(tenant, rName)
	if err != nil {
		return err
	}
	resp, err := scmCli.ShowCertificate(&scmmodel.ShowCertificateRequest{
		CertificateId: strings.TrimSpace(inst.ID),
	})
	if err != nil {
		return errors.Wrap(err, "ShowCertificate")
	}
	if resp == nil {
		return errors.New("ShowCertificate empty response")
	}
	if v := strPtrVal(resp.NotBefore); v != "" {
		inst.NotBefore = v
	}
	if v := strPtrVal(resp.NotAfter); v != "" {
		inst.NotAfter = v
	}
	if inst.NotAfter == "" && inst.NotBefore == "" {
		glog.Warningf("cert ShowCertificate: id=%s name=%q has empty not_before/not_after", inst.ID, inst.Name)
	}
	return nil
}

func strPtrVal(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func newHuaweiScmClient(tenant tenanter.Tenanter, endpointRegion string) (*hwscm.ScmClient, error) {
	t, ok := tenant.(*tenanter.AccessKeyTenant)
	if !ok {
		return nil, errors.New("not AccessKeyTenant")
	}
	rName := endpointRegion
	baseAuth := basic.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).Build()
	iamCli := hwiam.NewIamClient(hwiam.IamClientBuilder().
		WithRegion(huaweicloudregion.EndpointForService("iam", rName)).
		WithCredential(baseAuth).
		WithHttpConfig(huaweicloudregion.SDKHttpConfig()).
		Build())
	projResp, err := huaweicloudregion.KeystoneListProjectsResolveProject(iamCli, rName)
	if err != nil || projResp == nil || projResp.Projects == nil || len(*projResp.Projects) == 0 {
		if err == nil {
			err = errors.New("empty project list")
		}
		return nil, errors.Wrapf(err, "KeystoneListProjects region %s", rName)
	}
	projectID := (*projResp.Projects)[0].Id
	auth := basic.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).WithProjectId(projectID).Build()
	return hwscm.NewScmClient(hwscm.ScmClientBuilder().
		WithRegion(huaweicloudregion.EndpointForService("scm", rName)).
		WithCredential(auth).
		WithHttpConfig(huaweicloudregion.SDKHttpConfig()).
		Build()), nil
}
