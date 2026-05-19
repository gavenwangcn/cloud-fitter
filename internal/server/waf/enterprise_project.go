package waf

import (
	"os"
	"regexp"
	"strings"

	"github.com/golang/glog"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hweps "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/eps/v1"
	epsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/eps/v1/model"
	epsregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/eps/v1/region"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/internal/huaweicloudregion"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

var enterpriseProjectUUIDRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// enterpriseProjectSpec 供 WAF OpenAPI 使用的 enterprise_project_id（官方仅接受 0 / all_granted_eps / 36 位 UUID）。
type enterpriseProjectSpec struct {
	QueryID   string // 传给 WAF API 的 enterprise_project_id
	Name      string // 控制台展示名称（若已知）
	ConfigKey string // 配置来源说明，便于日志排查
}

func wafEnterpriseProjectNameFromEnv() string {
	if v := strings.TrimSpace(os.Getenv("CLOUD_FITTER_HUAWEI_WAF_ENTERPRISE_PROJECT_NAME")); v != "" {
		return v
	}
	return "dft_lc_infra"
}

func wafEnterpriseProjectIDFromEnv() string {
	return strings.TrimSpace(os.Getenv("CLOUD_FITTER_HUAWEI_WAF_ENTERPRISE_PROJECT_ID"))
}

func resolveWafEnterpriseProject(tenant *tenanter.AccessKeyTenant) (enterpriseProjectSpec, error) {
	idCfg := wafEnterpriseProjectIDFromEnv()
	nameCfg := wafEnterpriseProjectNameFromEnv()

	switch {
	case idCfg == "0" || idCfg == "all_granted_eps":
		return enterpriseProjectSpec{QueryID: idCfg, ConfigKey: "env_id"}, nil
	case enterpriseProjectUUIDRe.MatchString(idCfg):
		return enterpriseProjectSpec{QueryID: idCfg, ConfigKey: "env_id"}, nil
	case idCfg != "" && !enterpriseProjectUUIDRe.MatchString(idCfg):
		// 兼容误将企业项目名称写入 ID 环境变量
		glog.Warningf("waf enterprise_project_id env %q is not a UUID; resolve as enterprise project name via EPS", idCfg)
		return resolveEnterpriseProjectByName(tenant, idCfg, "env_id_as_name")
	case nameCfg != "":
		return resolveEnterpriseProjectByName(tenant, nameCfg, "env_name")
	default:
		return enterpriseProjectSpec{QueryID: "all_granted_eps", ConfigKey: "default_all_granted_eps"}, nil
	}
}

func resolveEnterpriseProjectByName(tenant *tenanter.AccessKeyTenant, name, configKey string) (enterpriseProjectSpec, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return enterpriseProjectSpec{}, errors.New("empty enterprise project name")
	}
	epsCli, err := newEpsClient(tenant)
	if err != nil {
		return enterpriseProjectSpec{}, err
	}
	limit := int32(1000)
	req := &epsmodel.ListEnterpriseProjectRequest{
		Name:  strPtr(name),
		Limit: &limit,
	}
	resp, err := epsCli.ListEnterpriseProject(req)
	if err != nil {
		return enterpriseProjectSpec{}, errors.Wrapf(err, "EPS ListEnterpriseProject name=%q", name)
	}
	if resp == nil || resp.EnterpriseProjects == nil || len(*resp.EnterpriseProjects) == 0 {
		return enterpriseProjectSpec{}, errors.Errorf("enterprise project not found: %q", name)
	}
	for i := range *resp.EnterpriseProjects {
		ep := &(*resp.EnterpriseProjects)[i]
		if strings.EqualFold(strings.TrimSpace(ep.Name), name) {
			glog.Infof("waf resolved enterprise project name=%q id=%q via EPS", ep.Name, ep.Id)
			return enterpriseProjectSpec{
				QueryID:   ep.Id,
				Name:      ep.Name,
				ConfigKey: configKey,
			}, nil
		}
	}
	// 模糊搜索可能返回多个；若无精确匹配则取第一条并告警
	ep := &(*resp.EnterpriseProjects)[0]
	glog.Warningf("waf enterprise project exact name %q not found; use first EPS match name=%q id=%q", name, ep.Name, ep.Id)
	return enterpriseProjectSpec{
		QueryID:   ep.Id,
		Name:      ep.Name,
		ConfigKey: configKey + "_fuzzy",
	}, nil
}

func newEpsClient(tenant *tenanter.AccessKeyTenant) (*hweps.EpsClient, error) {
	auth := basic.NewCredentialsBuilder().WithAk(tenant.GetId()).WithSk(tenant.GetSecret()).Build()
	return hweps.NewEpsClient(hweps.EpsClientBuilder().
		WithRegion(epsregion.CN_NORTH_4).
		WithCredential(auth).
		WithHttpConfig(huaweicloudregion.SDKHttpConfig()).
		Build()), nil
}
