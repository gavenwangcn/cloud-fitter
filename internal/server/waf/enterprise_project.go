package waf

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/golang/glog"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
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
	return strings.TrimSpace(os.Getenv("CLOUD_FITTER_HUAWEI_WAF_ENTERPRISE_PROJECT_NAME"))
}

func wafEnterpriseProjectIDFromEnv() string {
	if v := strings.TrimSpace(os.Getenv("CLOUD_FITTER_HUAWEI_WAF_ENTERPRISE_PROJECT_ID")); v != "" {
		return v
	}
	// 华为云默认企业项目 default，OpenAPI enterprise_project_id 固定为 "0"
	return "0"
}

func wafEnterpriseProjectFallbackAll() bool {
	v := strings.TrimSpace(os.Getenv("CLOUD_FITTER_HUAWEI_WAF_ENTERPRISE_PROJECT_FALLBACK"))
	if v == "" {
		return true
	}
	switch strings.ToLower(v) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func resolveWafEnterpriseProject(tenant *tenanter.AccessKeyTenant) (enterpriseProjectSpec, error) {
	idCfg := wafEnterpriseProjectIDFromEnv()
	nameCfg := wafEnterpriseProjectNameFromEnv()

	switch {
	case idCfg == "0":
		return enterpriseProjectSpec{QueryID: "0", Name: "default", ConfigKey: "env_id"}, nil
	case idCfg == "all_granted_eps":
		return enterpriseProjectSpec{QueryID: idCfg, ConfigKey: "env_id"}, nil
	case enterpriseProjectUUIDRe.MatchString(idCfg):
		return enterpriseProjectSpec{QueryID: idCfg, ConfigKey: "env_id"}, nil
	case idCfg != "" && !enterpriseProjectUUIDRe.MatchString(idCfg):
		glog.Warningf("waf enterprise_project_id env %q is not a UUID; resolve as enterprise project name via EPS", idCfg)
		return resolveEnterpriseProjectByName(tenant, idCfg, "env_id_as_name")
	case nameCfg != "":
		return resolveEnterpriseProjectByName(tenant, nameCfg, "env_name")
	default:
		return enterpriseProjectSpec{QueryID: "0", Name: "default", ConfigKey: "default_eps_0"}, nil
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

	all, err := listAllEnterpriseProjects(epsCli)
	if err != nil {
		return enterpriseProjectSpec{}, errors.Wrap(err, "EPS ListEnterpriseProject")
	}
	if len(all) == 0 {
		glog.Warningf("waf EPS returned 0 enterprise projects for name=%q; AK may lack eps:enterpriseProjects:list", name)
		if wafEnterpriseProjectFallbackAll() {
			return enterpriseProjectSpec{
				QueryID:   "all_granted_eps",
				Name:      name,
				ConfigKey: configKey + "_fallback_all_granted_eps",
			}, nil
		}
		return enterpriseProjectSpec{}, errors.Errorf("enterprise project not found: %q (EPS list empty)", name)
	}

	if ep := matchEnterpriseProject(all, name); ep != nil {
		glog.Infof("waf resolved enterprise project want=%q name=%q id=%q via EPS", name, ep.Name, ep.Id)
		return enterpriseProjectSpec{
			QueryID:   ep.Id,
			Name:      ep.Name,
			ConfigKey: configKey,
		}, nil
	}

	glog.Warningf("waf enterprise project %q not matched in EPS; available: %s",
		name, formatEnterpriseProjectList(all))
	if wafEnterpriseProjectFallbackAll() {
		glog.Warningf("waf fallback enterprise_project_id=all_granted_eps for want=%q", name)
		return enterpriseProjectSpec{
			QueryID:   "all_granted_eps",
			Name:      name,
			ConfigKey: configKey + "_fallback_all_granted_eps",
		}, nil
	}
	return enterpriseProjectSpec{}, errors.Errorf("enterprise project not found: %q", name)
}

func listAllEnterpriseProjects(epsCli *hweps.EpsClient) ([]epsmodel.EpDetail, error) {
	const limit int32 = 1000
	var (
		all    []epsmodel.EpDetail
		offset int32
	)
	for {
		req := &epsmodel.ListEnterpriseProjectRequest{
			Limit:  int32Ptr(limit),
			Offset: int32Ptr(offset),
		}
		resp, err := epsCli.ListEnterpriseProject(req)
		if err != nil {
			return nil, err
		}
		if resp == nil || resp.EnterpriseProjects == nil || len(*resp.EnterpriseProjects) == 0 {
			break
		}
		all = append(all, (*resp.EnterpriseProjects)...)
		if resp.TotalCount == nil || int32(len(all)) >= *resp.TotalCount {
			break
		}
		offset += limit
	}
	return all, nil
}

func matchEnterpriseProject(all []epsmodel.EpDetail, want string) *epsmodel.EpDetail {
	want = strings.TrimSpace(want)
	if want == "" {
		return nil
	}
	wantLower := strings.ToLower(want)
	norm := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.ReplaceAll(s, "_", "-")
		return s
	}
	wantNorm := norm(want)

	// 1. 名称精确匹配
	for i := range all {
		ep := &all[i]
		if strings.EqualFold(strings.TrimSpace(ep.Name), want) {
			return ep
		}
	}
	// 2. ID 精确匹配（误把 UUID 写在 name 里）
	for i := range all {
		ep := &all[i]
		if strings.EqualFold(strings.TrimSpace(ep.Id), want) {
			return ep
		}
	}
	// 3. 规范化名称（下划线/连字符）
	for i := range all {
		ep := &all[i]
		if norm(ep.Name) == wantNorm {
			return ep
		}
	}
	// 4. 名称包含
	var contains []*epsmodel.EpDetail
	for i := range all {
		ep := &all[i]
		n := strings.ToLower(strings.TrimSpace(ep.Name))
		if strings.Contains(n, wantLower) || strings.Contains(wantLower, n) {
			contains = append(contains, ep)
		}
	}
	if len(contains) == 1 {
		return contains[0]
	}
	return nil
}

func formatEnterpriseProjectList(all []epsmodel.EpDetail) string {
	parts := make([]string, 0, len(all))
	for i := range all {
		ep := &all[i]
		parts = append(parts, fmt.Sprintf("%s(%s)", ep.Name, ep.Id))
	}
	return strings.Join(parts, ", ")
}

func newEpsClient(tenant *tenanter.AccessKeyTenant) (*hweps.EpsClient, error) {
	auth := global.NewCredentialsBuilder().WithAk(tenant.GetId()).WithSk(tenant.GetSecret()).Build()
	return hweps.NewEpsClient(hweps.EpsClientBuilder().
		WithRegion(epsregion.CN_NORTH_4).
		WithCredential(auth).
		WithHttpConfig(huaweicloudregion.SDKHttpConfig()).
		Build()), nil
}
