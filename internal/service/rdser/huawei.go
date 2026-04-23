package rdser

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hwrds "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3/model"
	hwiam "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3"
	iammodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3/model"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbrds"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/huaweicloudregion"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

type HuaweiRds struct {
	cli      *hwrds.RdsClient
	region   tenanter.Region
	tenanter tenanter.Tenanter
}

func newHuaweiRdsClient(region tenanter.Region, tenant tenanter.Tenanter) (Rdser, error) {
	var (
		client *hwrds.RdsClient
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
		hcClient := hwrds.RdsClientBuilder().WithRegion(huaweicloudregion.EndpointForService("rds", rName)).WithCredential(auth).Build()
		client = hwrds.NewRdsClient(hcClient)
	default:
	}

	if err != nil {
		return nil, errors.Wrap(err, "init huawei rds client error")
	}
	return &HuaweiRds{
		cli:      client,
		region:   region,
		tenanter: tenant,
	}, nil
}

func huaweiRdsDatastoreTypeString(t model.DatastoreType) string {
	b, err := json.Marshal(t)
	if err != nil {
		return ""
	}
	return strings.Trim(string(b), `"`)
}

func (r *HuaweiRds) ListDetail(ctx context.Context, req *pbrds.ListDetailReq) (*pbrds.ListDetailResp, error) {
	request := new(model.ListInstancesRequest)
	offset := (req.PageNumber - 1) * req.PageSize
	request.Offset = &offset
	limit := req.PageSize
	request.Limit = &limit

	resp, err := r.cli.ListInstances(request)
	if err != nil {
		return nil, errors.Wrap(err, "Huawei RDS ListInstances error")
	}

	if resp.Instances == nil {
		return &pbrds.ListDetailResp{
			Rdses:      nil,
			Finished:   true,
			PageNumber: req.PageNumber + 1,
			PageSize:   req.PageSize,
			NextToken:  "",
			RequestId:  "",
		}, nil
	}

	instances := *resp.Instances
	rdses := make([]*pbrds.RdsInstance, len(instances))
	for k, v := range instances {
		engine := ""
		engineVersion := ""
		if v.Datastore != nil {
			engine = huaweiRdsDatastoreTypeString(v.Datastore.Type)
			engineVersion = v.Datastore.Version
		}
		cpu := int32(0)
		if v.Cpu != nil {
			if n, err := strconv.ParseInt(*v.Cpu, 10, 32); err == nil {
				cpu = int32(n)
			}
		}
		memMB := int32(0)
		if v.Mem != nil {
			if gb, err := strconv.ParseFloat(*v.Mem, 64); err == nil {
				memMB = int32(gb * 1024)
			}
		}
		charge := ""
		if v.ChargeInfo != nil {
			raw, err := json.Marshal(v.ChargeInfo.ChargeMode)
			if err == nil {
				charge = strings.Trim(string(raw), `"`)
			}
		}
		pub := append([]string(nil), v.PublicIps...)
		priv := append([]string(nil), v.PrivateIps...)
		rdses[k] = &pbrds.RdsInstance{
			Provider:      pbtenant.CloudProvider_huawei,
			AccoutName:    r.tenanter.AccountName(),
			InstanceId:    v.Id,
			InstanceName:  v.Name,
			RegionName:    r.region.GetName(),
			InstanceType:  v.Type,
			Engine:        engine,
			EngineVersion: engineVersion,
			InstanceClass: v.FlavorRef,
			Status:        v.Status,
			CreationTime:  v.Created,
			ExpireTime:    "",
			Cpu:           cpu,
			MemoryMb:      memMB,
			PublicIps:     pub,
			PrivateIps:    priv,
			VpcId:         v.VpcId,
			Port:          v.Port,
			ChargeType:    charge,
		}
	}

	isFinished := false
	if len(rdses) < int(req.PageSize) {
		isFinished = true
	}

	return &pbrds.ListDetailResp{
		Rdses:      rdses,
		Finished:   isFinished,
		PageNumber: req.PageNumber + 1,
		PageSize:   req.PageSize,
		NextToken:  "",
		RequestId:  "",
	}, nil
}
