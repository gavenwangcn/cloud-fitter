package ecser

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbecs"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

type AwsEcs struct {
	cli      *awsec2.Client
	region   tenanter.Region
	tenanter tenanter.Tenanter
}

type awsVolBrief struct {
	sizeGiB int32
	volType string
}

func newAwsEcsClient(region tenanter.Region, tenant tenanter.Tenanter) (Ecser, error) {
	var (
		client *awsec2.Client
		err    error
	)

	switch t := tenant.(type) {
	case *tenanter.AccessKeyTenant:
		cfg, err := config.LoadDefaultConfig(context.TODO(),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(t.GetId(), t.GetSecret(), "")),
			config.WithRegion(region.GetName()),
		)
		if err != nil {
			return nil, errors.Wrap(err, "LoadDefaultConfig aws ecs client error")
		}
		client = awsec2.NewFromConfig(cfg)
	default:
	}

	if err != nil {
		return nil, errors.Wrap(err, "init aws ec2 client error")
	}
	return &AwsEcs{
		cli:      client,
		region:   region,
		tenanter: tenant,
	}, nil
}

func uniqNonEmptyStrings(in []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func awsDiskFromBlockDevs(inst types.Instance, volMap map[string]awsVolBrief) (sysGB, dataGB int32, summary string) {
	root := aws.ToString(inst.RootDeviceName)
	var parts []string
	for _, bdm := range inst.BlockDeviceMappings {
		if bdm.Ebs == nil || bdm.Ebs.VolumeId == nil {
			continue
		}
		vid := *bdm.Ebs.VolumeId
		info, ok := volMap[vid]
		if !ok {
			continue
		}
		dev := aws.ToString(bdm.DeviceName)
		if dev == root {
			sysGB = info.sizeGiB
			parts = append(parts, fmt.Sprintf("系统卷 %s:%dGiB(%s)", dev, info.sizeGiB, info.volType))
		} else {
			dataGB += info.sizeGiB
			parts = append(parts, fmt.Sprintf("数据卷 %s:%dGiB(%s)", dev, info.sizeGiB, info.volType))
		}
	}
	return sysGB, dataGB, strings.Join(parts, "; ")
}

func (ecs *AwsEcs) ListDetail(ctx context.Context, req *pbecs.ListDetailReq) (*pbecs.ListDetailResp, error) {
	request := new(awsec2.DescribeInstancesInput)
	request.MaxResults = req.PageSize
	if req.NextToken != "" {
		request.NextToken = &req.NextToken
	}

	resp, err := ecs.cli.DescribeInstances(ctx, request)
	if err != nil {
		return nil, errors.Wrap(err, "Aws ListDetail error")
	}

	var volumeIDs []string
	for _, v := range resp.Reservations {
		for _, inst := range v.Instances {
			for _, bdm := range inst.BlockDeviceMappings {
				if bdm.Ebs != nil && bdm.Ebs.VolumeId != nil && *bdm.Ebs.VolumeId != "" {
					volumeIDs = append(volumeIDs, *bdm.Ebs.VolumeId)
				}
			}
		}
	}
	volumeIDs = uniqNonEmptyStrings(volumeIDs)
	volMap := make(map[string]awsVolBrief)
	for i := 0; i < len(volumeIDs); i += 200 {
		end := i + 200
		if end > len(volumeIDs) {
			end = len(volumeIDs)
		}
		batch := volumeIDs[i:end]
		dOut, dErr := ecs.cli.DescribeVolumes(ctx, &awsec2.DescribeVolumesInput{VolumeIds: batch})
		if dErr != nil {
			return nil, errors.Wrap(dErr, "Aws DescribeVolumes error")
		}
		for _, vol := range dOut.Volumes {
			vid := aws.ToString(vol.VolumeId)
			volMap[vid] = awsVolBrief{sizeGiB: vol.Size, volType: string(vol.VolumeType)}
		}
	}

	var ecses []*pbecs.EcsInstance
	for _, v := range resp.Reservations {
		for _, v2 := range v.Instances {
			imageID := ""
			if v2.ImageId != nil {
				imageID = *v2.ImageId
			}
			osType := ""
			if v2.Platform != "" {
				osType = string(v2.Platform)
			}
			pub := ""
			if v2.PublicIpAddress != nil {
				pub = *v2.PublicIpAddress
			}
			cpu := int32(0)
			if v2.CpuOptions != nil {
				cpu = v2.CpuOptions.CoreCount
			}
			status := ""
			if v2.State != nil {
				status = string(v2.State.Name)
			}
			sysGB, dataGB, dsum := awsDiskFromBlockDevs(v2, volMap)
			ecses = append(ecses, &pbecs.EcsInstance{
				Provider:         pbtenant.CloudProvider_aws,
				AccountName:      ecs.tenanter.AccountName(),
				InstanceId:       *v2.InstanceId,
				InstanceName:     "",
				RegionName:       ecs.region.GetName(),
				PublicIps:        []string{pub},
				InstanceType:     string(v2.InstanceType),
				Cpu:              cpu,
				Memory:           0,
				Description:      "",
				Status:           status,
				CreationTime:     "",
				ExpireTime:       "",
				ImageId:          imageID,
				ImageName:        "",
				OsType:           osType,
				OsBit:            "",
				SystemDiskSizeGb: sysGB,
				DataDiskTotalGb:  dataGB,
				DiskSummary:      dsum,
			})
		}
	}

	if resp.NextToken != nil && *resp.NextToken != "" {
		return &pbecs.ListDetailResp{
			Ecses:      ecses,
			Finished:   false,
			NextToken:  *resp.NextToken,
			PageNumber: req.PageNumber + 1,
			PageSize:   req.PageSize,
		}, nil
	}
	return &pbecs.ListDetailResp{
		Ecses:      ecses,
		Finished:   true,
		NextToken:  "",
		PageNumber: req.PageNumber + 1,
		PageSize:   req.PageSize,
	}, nil
}
