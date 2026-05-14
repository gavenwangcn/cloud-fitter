package kafkaer

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/golang/glog"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hwiam "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3"
	hwkafka "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/kafka/v2"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/kafka/v2/model"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbkafka"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/envtags"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweitags"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweicloudregion"
	"github.com/cloud-fitter/cloud-fitter/internal/server/scope"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

type HuaweiKafka struct {
	cli      *hwkafka.KafkaClient
	region   tenanter.Region
	tenanter tenanter.Tenanter
}

func newHuaweiKafkaClient(region tenanter.Region, tenant tenanter.Tenanter) (Kafkaer, error) {
	var (
		client *hwkafka.KafkaClient
		err    error
	)

	switch t := tenant.(type) {
	case *tenanter.AccessKeyTenant:
		auth := basic.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).Build()
		rName := region.GetName()
		cli := hwiam.IamClientBuilder().WithRegion(huaweicloudregion.EndpointForService("iam", rName)).WithCredential(auth).WithHttpConfig(huaweicloudregion.SDKHttpConfig()).Build()
		c := hwiam.NewIamClient(cli)
		r, err := huaweicloudregion.KeystoneListProjectsResolveProject(c, rName)
		if err != nil || r == nil || r.Projects == nil || len(*r.Projects) == 0 {
			if err == nil {
				err = errors.New("empty project list")
			}
			return nil, errors.Wrapf(err, "Huawei KeystoneListProjects regionName %s", rName)
		}
		projectId := (*r.Projects)[0].Id

		auth = basic.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).WithProjectId(projectId).Build()
		hcClient := hwkafka.KafkaClientBuilder().WithRegion(huaweicloudregion.EndpointForService("dms", rName)).WithCredential(auth).WithHttpConfig(huaweicloudregion.SDKHttpConfig()).Build()
		client = hwkafka.NewKafkaClient(hcClient)
	default:
	}

	if err != nil {
		return nil, errors.Wrap(err, "init huawei kafka client error")
	}
	return &HuaweiKafka{
		cli:      client,
		region:   region,
		tenanter: tenant,
	}, nil
}

func huaweiKafkaSecurityGroupNames(v *model.ShowInstanceResp) []string {
	if v == nil {
		return nil
	}
	if v.SecurityGroupName != nil {
		s := strings.TrimSpace(*v.SecurityGroupName)
		if s != "" {
			return []string{s}
		}
	}
	if v.SecurityGroupId != nil {
		s := strings.TrimSpace(*v.SecurityGroupId)
		if s != "" {
			return []string{s}
		}
	}
	return nil
}

func (kafka *HuaweiKafka) ListDetail(ctx context.Context, req *pbkafka.ListDetailReq) (*pbkafka.ListDetailResp, error) {
	request := new(model.ListInstancesRequest)
	request.Engine = model.GetListInstancesRequestEngineEnum().KAFKA
	// v0.0.40-rc 的 ListInstancesRequest 无 offset/limit；ListInstances 单次返回当前过滤下全量

	resp, err := huaweicloudregion.DoWithTransientNetworkRetry(func() (*model.ListInstancesResponse, error) {
		return kafka.cli.ListInstances(request)
	})
	if err != nil {
		return nil, errors.Wrap(err, "Huawei ListDetail error")
	}

	instances := *resp.Instances
	n := len(instances)
	type kafkaTagFetch struct {
		pairs [][2]string
		err   error
	}
	tagFetches := make([]kafkaTagFetch, n)
	var tagOK, tagFail int32
	glog.Infof("Huawei Kafka ListDetail ShowKafkaTags pull begin account=%s region=%s instances=%d",
		kafka.tenanter.AccountName(), kafka.region.GetName(), n)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)
	for ti := 0; ti < n; ti++ {
		ti := ti
		iv := instances[ti]
		if iv.InstanceId == nil || strings.TrimSpace(*iv.InstanceId) == "" {
			continue
		}
		iid := strings.TrimSpace(*iv.InstanceId)
		wg.Add(1)
		go func(ti int, iid string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			tr, err := huaweicloudregion.DoWithTransientNetworkRetry(func() (*model.ShowKafkaTagsResponse, error) {
				return kafka.cli.ShowKafkaTags(&model.ShowKafkaTagsRequest{InstanceId: iid})
			})
			if err != nil {
				atomic.AddInt32(&tagFail, 1)
				tagFetches[ti].err = err
				glog.Warningf("Huawei Kafka ShowKafkaTags failed instance_id=%s account=%s region=%s err=%v (需 IAM 含 kafka:instance:listTags 或等价只读)",
					iid, kafka.tenanter.AccountName(), kafka.region.GetName(), err)
				return
			}
			atomic.AddInt32(&tagOK, 1)
			tagFetches[ti].pairs = huaweitags.PairsFromKafkaTagEntities(tr.Tags)
		}(ti, iid)
	}
	wg.Wait()
	glog.Infof("Huawei Kafka ListDetail ShowKafkaTags pull end account=%s region=%s ok=%d fail=%d",
		kafka.tenanter.AccountName(), kafka.region.GetName(), tagOK, tagFail)

	var kafkas []*pbkafka.KafkaInstance
	for i, v := range instances {
		if v.InstanceId == nil || v.Name == nil || v.Status == nil || v.CreatedAt == nil {
			continue
		}
		listPairs := huaweitags.PairsFromKafkaTagEntities(v.Tags)
		var merged [][2]string
		if tagFetches[i].err == nil && len(tagFetches[i].pairs) > 0 {
			merged = huaweitags.MergePairsPreferPrimary(tagFetches[i].pairs, listPairs)
		} else {
			merged = listPairs
		}
		if !scope.SystemListTagFilterMatches(ctx, envtags.FromPairs(envtags.SystemTagKey(), merged)) {
			continue
		}
		userDisp := huaweitags.FilterPairsExcludingHuaweiSysPrefix(merged)
		kafkas = append(kafkas, &pbkafka.KafkaInstance{
			Provider:             pbtenant.CloudProvider_huawei,
			AccoutName:           kafka.tenanter.AccountName(),
			InstanceId:           *v.InstanceId,
			InstanceName:         *v.Name,
			RegionName:           kafka.region.GetName(),
			EndPoint:             "",
			TopicNumLimit:        0,
			DistSize:             0,
			Status:               *v.Status,
			CreateTime:           *v.CreatedAt,
			ExpiredTime:          "",
			NodeTagValue:         envtags.FromPairs(envtags.NodeTagKey(), merged),
			SecurityGroupNames:   huaweiKafkaSecurityGroupNames(&v),
			SystemTagsDisplay:    strings.TrimSpace(envtags.FromPairs(envtags.SystemTagKey(), merged)),
			UserTagsDisplay:      huaweitags.FormatPairsDisplay(userDisp),
		})
	}

	return &pbkafka.ListDetailResp{
		Kafkas:     kafkas,
		Finished:   true,
		NextToken:  "",
		PageNumber: req.PageNumber + 1,
		PageSize:   req.PageSize,
		RequestId:  "",
	}, nil
}
