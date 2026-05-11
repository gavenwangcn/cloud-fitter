package kafkaer

import (
	"context"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hwiam "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3"
	iammodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3/model"
	hwkafka "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/kafka/v2"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/kafka/v2/model"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbkafka"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweicloudregion"
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
		hcClient := hwkafka.KafkaClientBuilder().WithRegion(huaweicloudregion.EndpointForService("dms", rName)).WithCredential(auth).Build()
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

func (kafka *HuaweiKafka) ListDetail(ctx context.Context, req *pbkafka.ListDetailReq) (*pbkafka.ListDetailResp, error) {
	request := new(model.ListInstancesRequest)
	request.Engine = model.GetListInstancesRequestEngineEnum().KAFKA
	// v0.0.40-rc 的 ListInstancesRequest 无 offset/limit；ListInstances 单次返回当前过滤下全量

	resp, err := kafka.cli.ListInstances(request)
	if err != nil {
		return nil, errors.Wrap(err, "Huawei ListDetail error")
	}

	instances := *resp.Instances
	var kafkas = make([]*pbkafka.KafkaInstance, len(instances))
	for k, v := range instances {
		kafkas[k] = &pbkafka.KafkaInstance{
			Provider:      pbtenant.CloudProvider_huawei,
			AccoutName:    kafka.tenanter.AccountName(),
			InstanceId:    *v.InstanceId,
			InstanceName:  *v.Name,
			RegionName:    kafka.region.GetName(),
			EndPoint:      "",
			TopicNumLimit: 0,
			DistSize:      0,
			Status:        *v.Status,
			CreateTime:    *v.CreatedAt,
			ExpiredTime:   "",
		}
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
