package huaweices

import (
	"context"
	"math"
	"strings"

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

// DimPair CES 指标除首维度外的其它维度（如 AGT.ECS 的 disk_usedPercent 需 mount_point）。
type DimPair struct {
	Name  string
	Value string
}

// MetricQuery 单个指标：首维度 DimName/DimValue + 可选 ExtraDims；mapKey 与响应侧按维度取值顺序拼接一致。
type MetricQuery struct {
	Namespace  string
	DimName    string
	DimValue   string
	ExtraDims  []DimPair
	MetricName string
}

func (q MetricQuery) mapKey() string {
	parts := []string{q.DimValue}
	for _, e := range q.ExtraDims {
		parts = append(parts, e.Value)
	}
	return strings.Join(parts, "\x00") + "\x00" + q.MetricName
}

// BatchQueryAverageSeries 调用华为云监控 CES 官方接口批量查询监控数据：
// OpenAPI「Querying Monitoring Data of Multiple Metrics」— POST /V1.0/{project_id}/batch-query-metric-data
// 文档：https://support.huaweicloud.com/intl/en-us/api-ces/ces_03_0034.html
// SDK：CesClient.BatchListMetricData。本函数固定 filter=average、period 见 Period1Hour。
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
			dims := make([]cesmodel.MetricsDimension, 0, 1+len(q.ExtraDims))
			dims = append(dims, cesmodel.MetricsDimension{Name: q.DimName, Value: &v})
			for _, e := range q.ExtraDims {
				ev := e.Value
				en := e.Name
				dims = append(dims, cesmodel.MetricsDimension{Name: en, Value: &ev})
			}
			metrics = append(metrics, cesmodel.MetricInfo{
				Namespace: q.Namespace, MetricName: q.MetricName, Dimensions: dims,
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
			var keyParts []string
			if m.Dimensions != nil {
				for _, d := range *m.Dimensions {
					if d.Value != nil && *d.Value != "" {
						keyParts = append(keyParts, *d.Value)
					}
				}
			}
			k := strings.Join(keyParts, "\x00") + "\x00" + m.MetricName
			out[k] = append(out[k], m.Datapoints...)
		}
	}
	return out, nil
}

// RoundPercent2 将百分比（0–100）四舍五入到至多两位小数，供 API JSON 展示。
func RoundPercent2(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	return math.Round(x*100) / 100
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

// PeakAvgFromAveragePoints 仅返回峰值与算术均值（不计算最小值）。
func PeakAvgFromAveragePoints(dps []cesmodel.DatapointForBatchMetric) (peak, avg float64, ok bool) {
	peak, avg, _, ok = PeakAvgMinFromAveragePoints(dps)
	return peak, avg, ok
}

// AvgFromAveragePoints 仅返回期间算术平均利用率（与 UtilizationWindow 中 avg 口径一致）。
func AvgFromAveragePoints(dps []cesmodel.DatapointForBatchMetric) (avg float64, ok bool) {
	_, avg, _, ok = PeakAvgMinFromAveragePoints(dps)
	return avg, ok
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
