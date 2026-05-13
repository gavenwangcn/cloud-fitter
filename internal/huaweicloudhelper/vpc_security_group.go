package huaweicloudhelper

import (
	"strings"

	"github.com/golang/glog"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hwiam "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3"
	hwvpc "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/vpc/v3"
	vpcmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/vpc/v3/model"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/internal/huaweicloudregion"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

// NewVPCClient 与 RDS/DCS 等共用：同一套 AK/SK + 区域 project。
func NewVPCClient(region tenanter.Region, tenant tenanter.Tenanter) (*hwvpc.VpcClient, error) {
	switch t := tenant.(type) {
	case *tenanter.AccessKeyTenant:
		auth := basic.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).Build()
		rName := region.GetName()
		cli := hwiam.IamClientBuilder().WithRegion(huaweicloudregion.EndpointForService("iam", rName)).WithCredential(auth).WithHttpConfig(huaweicloudregion.SDKHttpConfig()).Build()
		c := hwiam.NewIamClient(cli)
		proj, err := huaweicloudregion.KeystoneListProjectsResolveProject(c, rName)
		if err != nil || proj == nil || proj.Projects == nil || len(*proj.Projects) == 0 {
			if err == nil {
				err = errors.New("empty project list")
			}
			return nil, errors.Wrapf(err, "Huawei KeystoneListProjects regionName %s", rName)
		}
		projectId := (*proj.Projects)[0].Id

		auth = basic.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).WithProjectId(projectId).Build()
		hcClient := hwvpc.VpcClientBuilder().WithRegion(huaweicloudregion.EndpointForService("vpc", rName)).WithCredential(auth).WithHttpConfig(huaweicloudregion.SDKHttpConfig()).Build()
		return hwvpc.NewVpcClient(hcClient), nil
	default:
	}
	return nil, errors.New("init huawei vpc client: unsupported tenant")
}

// LookupSecurityGroupDisplayNames 按安全组 ID 调用 VPC ShowSecurityGroup，返回 id->可读名称（失败的不出现在 map 中）。
func LookupSecurityGroupDisplayNames(cli *hwvpc.VpcClient, ids []string) map[string]string {
	if cli == nil || len(ids) == 0 {
		return nil
	}
	out := make(map[string]string)
	seen := make(map[string]struct{})
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		req := &vpcmodel.ShowSecurityGroupRequest{SecurityGroupId: id}
		resp, err := huaweicloudregion.DoWithTransientNetworkRetry(func() (*vpcmodel.ShowSecurityGroupResponse, error) {
			return cli.ShowSecurityGroup(req)
		})
		if err != nil || resp == nil || resp.SecurityGroup == nil {
			glog.V(2).Infof("Huawei ShowSecurityGroup id=%s err=%v", id, err)
			continue
		}
		nm := strings.TrimSpace(resp.SecurityGroup.Name)
		if nm != "" {
			out[id] = nm
		}
	}
	return out
}
