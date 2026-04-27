package huaweices

import (
	"context"
	"math"

	"github.com/golang/glog"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hwces "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1"
	cesmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1/model"
	hwiam "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3"
	iammodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3/model"
	"github.com/pkg/errors"

	"github.com/cloud-fitter/cloud-fitter/internal/huaweicloudregion"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

// Huawei CES 批量接口单请求最多 10 个指标（见 BatchListMetricDataRequestBody.Metrics 注释）。
const MaxMetricsPerBatch = 10

// Period1Hour 聚合粒度：3600 秒，小时级数据点（文档 period 取值）。
const Period1Hour = "3600"

// NewClient 与 ECS/RDS 相同：解析区域对应 project_id 后创建 CES 客户端。
func NewClient(regionName string, tenant tenanter.Tenanter) (*hwces.CesClient, error) {
	t, ok := tenant.(*tenanter.AccessKeyTenant)
	if !ok {
		return nil, errors.New("huaweices: only AccessKeyTenant supported")
	}
	auth := basic.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).Build()
	cli := hwiam.IamClientBuilder().WithRegion(huaweicloudregion.EndpointForService("iam", regionName)).WithCredential(auth).Build()
	c := hwiam.NewIamClient(cli)
	req := new(iammodel.KeystoneListProjectsRequest)
	req.Name = &regionName
	r, err := c.KeystoneListProjects(req)
	if err != nil || r.Projects == nil || len(*r.Projects) == 0 {
		return nil, errors.Wrapf(err, "huaweices KeystoneListProjects region=%s", regionName)
	}
	projectID := (*r.Projects)[0].Id
	auth = basic.NewCredentialsBuilder().WithAk(t.GetId()).WithSk(t.GetSecret()).WithProjectId(projectID).Build()
	hc := hwces.CesClientBuilder().WithRegion(huaweicloudregion.EndpointForService("ces", regionName)).WithCredential(auth).Build()
	return hwces.NewCesClient(hc), nil
}

// MetricQuery 单个指标维度 + 名称。
type MetricQuery struct {
	Namespace  string
	DimName    string
	DimValue   string
	MetricName string
}

func (q MetricQuery) mapKey() string {
	return q.DimValue + "\x00" + q.MetricName
}

// BatchQueryAverageSeries 使用 filter=average 拉取时间序列，返回 mapKey -> 数据点（各点 Average 字段有效）。
func BatchQueryAverageSeries(ctx context.Context, cli *hwces.CesClient, queries []MetricQuery, fromMs, toMs int64) (map[string][]cesmodel.DatapointForBatchMetric, error) {
	if len(queries) == 0 {
		return map[string][]cesmodel.DatapointForBatchMetric{}, nil
	}
	out := make(map[string][]cesmodel.DatapointForBatchMetric)
	for start := 0; start < len(queries); start += MaxMetricsPerBatch {
		end := start + MaxMetricsPerBatch
		if end > len(queries) {
			end = len(queries)
		}
		chunk := queries[start:end]
		metrics := make([]cesmodel.MetricInfo, 0, len(chunk))
		for _, q := range chunk {
			v := q.DimValue
			metrics = append(metrics, cesmodel.MetricInfo{
				Namespace:  q.Namespace,
				MetricName: q.MetricName,
				Dimensions: []cesmodel.MetricsDimension{
					{Name: q.DimName, Value: &v},
				},
			})
		}
		body := &cesmodel.BatchListMetricDataRequestBody{
			Metrics: metrics,
			Period:  Period1Hour,
			Filter:  "average",
			From:    fromMs,
			To:      toMs,
		}
		req := &cesmodel.BatchListMetricDataRequest{Body: body}
		resp, err := cli.BatchListMetricData(req)
		if err != nil {
			return nil, errors.Wrap(err, "BatchListMetricData")
		}
		if resp.Metrics == nil {
			continue
		}
		for _, m := range *resp.Metrics {
			dimVal := ""
			if m.Dimensions != nil {
				for _, d := range *m.Dimensions {
					if d.Value != nil && *d.Value != "" {
						dimVal = *d.Value
						break
					}
				}
			}
			k := dimVal + "\x00" + m.MetricName
			out[k] = append(out[k], m.Datapoints...)
		}
	}
	return out, nil
}

// PeakAvgMinFromAveragePoints 对「各周期平均值」序列再求峰值、算术均值、谷值（百分比指标，0–100）。
func PeakAvgMinFromAveragePoints(dps []cesmodel.DatapointForBatchMetric) (peak, avg, min float64, ok bool) {
	if len(dps) == 0 {
		return 0, 0, 0, false
	}
	var sum float64
	n := 0
	first := true
	for _, dp := range dps {
		if dp.Average == nil {
			continue
		}
		v := *dp.Average
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		n++
		sum += v
		if first {
			peak, min = v, v
			first = false
		} else {
			if v > peak {
				peak = v
			}
			if v < min {
				min = v
			}
		}
	}
	if n == 0 {
		return 0, 0, 0, false
	}
	return peak, sum / float64(n), min, true
}

// ChunkByPairBudget 将实例 ID 列表按「每实例占 metricsPerInstance 个指标、每批最多 maxMetrics」分批。
func ChunkInstanceIDs(ids []string, metricsPerInstance, maxMetrics int) [][]string {
	if metricsPerInstance <= 0 || maxMetrics <= 0 {
		return nil
	}
	perBatch := maxMetrics / metricsPerInstance
	if perBatch < 1 {
		perBatch = 1
	}
	var batches [][]string
	for i := 0; i < len(ids); i += perBatch {
		j := i + perBatch
		if j > len(ids) {
			j = len(ids)
		}
		batches = append(batches, ids[i:j])
	}
	return batches
}

// LogBatchError 统一日志，避免 ListDetail 重复代码。
func LogBatchError(resource string, account, region string, err error) {
	glog.Warningf("Huawei CES batch query failed resource=%s account=%s region=%s err=%v",
		resource, account, region, err)
}
