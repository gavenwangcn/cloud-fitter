package ccer

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	ccemodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cce/v3/model"
	hwcc "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cce/v3"
	ecsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	hwecs "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2"
	hwiam "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3"
	iammodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3/model"
	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbcce"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/envtags"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweicloudregion"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

type HuaweiCce struct {
	cceCli   *hwcc.CceClient
	ecsCli   *hwecs.EcsClient
	region   tenanter.Region
	tenanter tenanter.Tenanter
}

func newHuaweiCceClient(region tenanter.Region, tenant tenanter.Tenanter) (Ccer, error) {
	var (
		cceClient *hwcc.CceClient
		ecsClient *hwecs.EcsClient
	)

	switch t := tenant.(type) {
	case *tenanter.AccessKeyTenant:
		auth := basic.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).Build()
		rName := region.GetName()
		cli := hwiam.IamClientBuilder().WithRegion(huaweicloudregion.EndpointForService("iam", rName)).WithCredential(auth).Build()
		c := hwiam.NewIamClient(cli)
		request := new(iammodel.KeystoneListProjectsRequest)
		request.Name = &rName
		r, err := c.KeystoneListProjects(request)
		if err != nil || len(*r.Projects) == 0 {
			return nil, errors.Wrapf(err, "Huawei KeystoneListProjects regionName %s", rName)
		}
		projectId := (*r.Projects)[0].Id

		auth = basic.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).WithProjectId(projectId).Build()
		hcCce := hwcc.CceClientBuilder().WithRegion(huaweicloudregion.EndpointForService("cce", rName)).WithCredential(auth).Build()
		cceClient = hwcc.NewCceClient(hcCce)
		hcEcs := hwecs.EcsClientBuilder().WithRegion(huaweicloudregion.EndpointForService("ecs", rName)).WithCredential(auth).Build()
		ecsClient = hwecs.NewEcsClient(hcEcs)
	default:
	}

	if cceClient == nil || ecsClient == nil {
		return nil, errors.New("init huawei cce client: unsupported tenant type")
	}
	return &HuaweiCce{
		cceCli:   cceClient,
		ecsCli:   ecsClient,
		region:   region,
		tenanter: tenant,
	}, nil
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func ecsFlavorVCPU(f *ecsmodel.ServerFlavor) int32 {
	if f == nil {
		return 0
	}
	n, err := strconv.ParseInt(f.Vcpus, 10, 32)
	if err != nil {
		return 0
	}
	return int32(n)
}

func ecsFlavorRAMMB(f *ecsmodel.ServerFlavor) int32 {
	if f == nil {
		return 0
	}
	n, err := strconv.ParseInt(f.Ram, 10, 32)
	if err != nil {
		return 0
	}
	return int32(n)
}

func cceNodePhaseString(p *ccemodel.V3NodeStatusPhase) string {
	if p == nil {
		return ""
	}
	b, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	return strings.Trim(string(b), `"`)
}

func cceNodePhaseNormal(phase string) bool {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "active", "installed":
		return true
	default:
		return false
	}
}

// ecsFlavorByServerID 分页拉取当前项目下 ECS，用于将 CCE 节点的 serverId 映射到规格（vCPU / 内存）。
func (c *HuaweiCce) ecsFlavorByServerID() (map[string]*ecsmodel.ServerFlavor, error) {
	out := make(map[string]*ecsmodel.ServerFlavor)
	var offset int32
	const limit int32 = 100
	for {
		off := offset
		lim := limit
		req := &ecsmodel.ListServersDetailsRequest{Offset: &off, Limit: &lim}
		resp, err := c.ecsCli.ListServersDetails(req)
		if err != nil {
			return nil, errors.Wrap(err, "Huawei ECS ListServersDetails")
		}
		if resp.Servers == nil {
			break
		}
		servers := *resp.Servers
		for i := range servers {
			s := servers[i]
			out[s.Id] = s.Flavor
		}
		if len(servers) < int(limit) {
			break
		}
		offset += limit
	}
	return out, nil
}

type cceNodeAgg struct {
	total, normal int32
	cpuTotal      int32
	memTotalMB    int32
}

func (c *HuaweiCce) clusterNodeAgg(clusterID string, flavorByID map[string]*ecsmodel.ServerFlavor) cceNodeAgg {
	var out cceNodeAgg
	if clusterID == "" {
		return out
	}
	req := ccemodel.ListNodesRequest{ClusterId: clusterID}
	resp, err := c.cceCli.ListNodes(&req)
	if err != nil {
		glog.Warningf("Huawei CCE ListNodes cluster_id=%s: %v", clusterID, err)
		return out
	}
	if resp.Items == nil {
		return out
	}
	for _, node := range *resp.Items {
		out.total++
		if node.Status == nil {
			continue
		}
		if cceNodePhaseNormal(cceNodePhaseString(node.Status.Phase)) {
			out.normal++
		}
		sid := derefStr(node.Status.ServerId)
		if sid == "" {
			continue
		}
		fl := flavorByID[sid]
		out.cpuTotal += ecsFlavorVCPU(fl)
		out.memTotalMB += ecsFlavorRAMMB(fl)
	}
	return out
}

// MapEcsInstanceIDToClusterUID 使用华为 CCE 公开接口：
// - GET /api/v3/projects/{project_id}/clusters
// - GET /api/v3/projects/{project_id}/clusters/{cluster_id}/nodes
// 其中 path 的 cluster_id 为集群资源 UUID，与 ListClusters 返回的 metadata.uid 一致；节点 status.serverId 为 ECS 云服务器 ID（与 ECS List 的实例 id 一致）。
func (c *HuaweiCce) MapEcsInstanceIDToClusterUID(ctx context.Context) (map[string]string, error) {
	_ = ctx
	r := new(ccemodel.ListClustersRequest)
	clustersResp, err := c.cceCli.ListClusters(r)
	if err != nil {
		return nil, errors.Wrap(err, "Huawei CCE ListClusters")
	}
	out := make(map[string]string)
	if clustersResp == nil || clustersResp.Items == nil {
		return out, nil
	}
	for _, cl := range *clustersResp.Items {
		if cl.Metadata == nil {
			continue
		}
		clusterUID := derefStr(cl.Metadata.Uid)
		if clusterUID == "" {
			glog.V(1).Infof("Huawei CCE: skip cluster with empty metadata.uid name=%q account=%s region=%s",
				cl.Metadata.Name, c.tenanter.AccountName(), c.region.GetName())
			continue
		}
		nodeReq := ccemodel.ListNodesRequest{ClusterId: clusterUID}
		nodeResp, err := c.cceCli.ListNodes(&nodeReq)
		if err != nil {
			return nil, errors.Wrapf(err, "Huawei CCE ListNodes cluster_id=%s", clusterUID)
		}
		if nodeResp == nil || nodeResp.Items == nil {
			continue
		}
		for _, node := range *nodeResp.Items {
			if node.Status == nil {
				continue
			}
			sid := derefStr(node.Status.ServerId)
			if sid == "" {
				continue
			}
			if prev, ok := out[sid]; ok && prev != clusterUID {
				glog.Warningf("Huawei CCE: ECS %s associated with more than one cluster (was %s, now %s) account=%s region=%s",
					sid, prev, clusterUID, c.tenanter.AccountName(), c.region.GetName())
			}
			out[sid] = clusterUID
		}
	}
	return out, nil
}

func (c *HuaweiCce) ListDetail(ctx context.Context, req *pbcce.ListDetailReq) (*pbcce.ListDetailResp, error) {
	r := new(ccemodel.ListClustersRequest)
	resp, err := c.cceCli.ListClusters(r)
	if err != nil {
		return nil, errors.Wrap(err, "Huawei CCE ListClusters error")
	}

	flavorByID, ferr := c.ecsFlavorByServerID()
	if ferr != nil {
		return nil, ferr
	}

	var clusters []*pbcce.CceCluster
	if resp.Items != nil {
		for _, cl := range *resp.Items {
			var name, uid, flavor, version, phase string
			if cl.Metadata != nil {
				name = cl.Metadata.Name
				uid = derefStr(cl.Metadata.Uid)
			}
			if cl.Spec != nil {
				flavor = cl.Spec.Flavor
				version = derefStr(cl.Spec.Version)
			}
			if cl.Status != nil {
				phase = derefStr(cl.Status.Phase)
			}
			agg := c.clusterNodeAgg(uid, flavorByID)
			var nodeTag string
			if cl.Metadata != nil && cl.Metadata.Labels != nil {
				nodeTag = envtags.FromMap(envtags.NodeTagKey(), cl.Metadata.Labels)
			}
			clusters = append(clusters, &pbcce.CceCluster{
				Provider:       pbtenant.CloudProvider_huawei,
				AccoutName:     c.tenanter.AccountName(),
				RegionName:     c.region.GetName(),
				ClusterName:    name,
				ClusterUid:     uid,
				Flavor:         flavor,
				K8SVersion:     version,
				Phase:          phase,
				NodeTotal:      agg.total,
				NodeNormal:     agg.normal,
				CpuTotal:       agg.cpuTotal,
				MemoryTotalMb:  agg.memTotalMB,
				NodeTagValue:   nodeTag,
			})
		}
	}

	return &pbcce.ListDetailResp{
		Clusters:   clusters,
		Finished:   true,
		PageNumber: req.PageNumber + 1,
		PageSize:   req.PageSize,
		NextToken:  "",
		RequestId:  "",
	}, nil
}
