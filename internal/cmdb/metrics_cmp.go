package cmdb

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// anyToCompareStr 将 CMDB JSON 中 cpu_count / ram_size 等标量与云上值对齐为可比较字符串。
func anyToCompareStr(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case int:
		return strconv.Itoa(t)
	case int32:
		return fmt.Sprintf("%d", t)
	case int64:
		return fmt.Sprintf("%d", t)
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return ""
		}
		if t == 0 {
			return "0"
		}
		if math.Floor(t) == t {
			return fmt.Sprintf("%.0f", t)
		}
		return strings.TrimSpace(strconv.FormatFloat(t, 'f', -1, 64))
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

// metricStrEqual 比较两个规格字符串，容忍 "4" 与 "4.0" 等差异。
func metricStrEqual(a, b string) bool {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if a == b {
		return true
	}
	if a == "" && b == "" {
		return true
	}
	fa, e1 := strconv.ParseFloat(a, 64)
	fb, e2 := strconv.ParseFloat(b, 64)
	if e1 == nil && e2 == nil {
		return math.Abs(fa-fb) < 1e-6
	}
	return false
}

// cmdbFloatMetricJSON 将主机/中间件利用率等字段写入 CMDB「浮点数」属性：JSON 中为 number 类型。
// 云上经 FormatFloat 得到的整数或小数串均可解析；空串保持空串（服务端对非 TEXT 空串视为清空）。
// 解析失败时退回空串，避免 PUT/POST 400。
func cmdbFloatMetricJSON(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return ""
	}
	return f
}

// cmdbSecurityGroupStr 读取 CMDB CI 中 security_group（字符串或多值列表均可），与 joinSecurityGroupsForCMDB 结果对齐比较。
func cmdbSecurityGroupStr(row map[string]any) string {
	v := row["security_group"]
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case []any:
		var parts []string
		for _, x := range t {
			s := strings.TrimSpace(anyToCompareStr(x))
			if s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "、")
	default:
		return strings.TrimSpace(anyToCompareStr(v))
	}
}

func serverResourceChanged(row map[string]any, h hostRec) bool {
	ct, locn := cmdbCloudLocationFromSysNodeName(strings.TrimSpace(h.SysNodeName), strings.TrimSpace(h.CloudLabel), strings.TrimSpace(h.Region))
	if strings.TrimSpace(anyToCompareStr(row["sys_node_name"])) != strings.TrimSpace(h.SysNodeName) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["server_name"])) != strings.TrimSpace(h.Name) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["private_ip"])) != strings.TrimSpace(h.IP) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["os_version"])) != strings.TrimSpace(h.OS) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["location"])) != strings.TrimSpace(locn) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["cloud_type"])) != strings.TrimSpace(ct) {
		return true
	}
	oCPU := anyToCompareStr(row["cpu_count"])
	nCPU := anyToCompareStr(int(h.CPU))
	oMem := anyToCompareStr(row["ram_size"])
	nMem := strings.TrimSpace(h.MemGBStr)
	oDisk := anyToCompareStr(row["disk_size"])
	nDisk := strings.TrimSpace(h.DiskStr)
	if !metricStrEqual(oCPU, nCPU) {
		return true
	}
	if !metricStrEqual(oMem, nMem) {
		return true
	}
	if !metricStrEqual(oDisk, nDisk) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["storage"])) != strings.TrimSpace(h.StorageAttr) {
		return true
	}
	if !metricStrEqual(anyToCompareStr(row["cpu_peak_30"]), strings.TrimSpace(h.CpuPeak30)) {
		return true
	}
	if !metricStrEqual(anyToCompareStr(row["cpu_peak_180"]), strings.TrimSpace(h.CpuPeak180)) {
		return true
	}
	if !metricStrEqual(anyToCompareStr(row["cpu_avg_30"]), strings.TrimSpace(h.CpuAvg30)) {
		return true
	}
	if !metricStrEqual(anyToCompareStr(row["cpu_avg_180"]), strings.TrimSpace(h.CpuAvg180)) {
		return true
	}
	if !metricStrEqual(anyToCompareStr(row["mem_peak_30"]), strings.TrimSpace(h.MemPeak30)) {
		return true
	}
	if !metricStrEqual(anyToCompareStr(row["men_peak_180"]), strings.TrimSpace(h.MenPeak180)) {
		return true
	}
	if !metricStrEqual(anyToCompareStr(row["men_avg_30"]), strings.TrimSpace(h.MemAvg30)) {
		return true
	}
	if !metricStrEqual(anyToCompareStr(row["men_avg_180"]), strings.TrimSpace(h.MenAvg180)) {
		return true
	}
	if !metricStrEqual(anyToCompareStr(row["disk_usage_30"]), strings.TrimSpace(h.DiskUsage30)) {
		return true
	}
	if !metricStrEqual(anyToCompareStr(row["disk_usage_180"]), strings.TrimSpace(h.DiskUsage180)) {
		return true
	}
	if strings.TrimSpace(cmdbSecurityGroupStr(row)) != strings.TrimSpace(h.SecurityGroup) {
		return true
	}
	return false
}

func middlewareResourceChanged(row map[string]any, m mwRec) bool {
	if strings.TrimSpace(anyToCompareStr(row["instance_id"])) != strings.TrimSpace(m.InstanceID) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["sys_node_name"])) != strings.TrimSpace(m.SysNodeName) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["environment"])) != strings.TrimSpace(m.Environment) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["resource_name"])) != strings.TrimSpace(m.Name) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["resource_type"])) != strings.TrimSpace(m.MwType) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["private_ip"])) != strings.TrimSpace(m.IP) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["location"])) != strings.TrimSpace(m.CloudLabel) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["cloud_type"])) != strings.TrimSpace(m.Region) {
		return true
	}
	oCPU := anyToCompareStr(row["cpu_count"])
	nCPU := strings.TrimSpace(m.CPU)
	oMem := anyToCompareStr(row["ram_size"])
	nMem := strings.TrimSpace(m.Mem)
	oDisk := anyToCompareStr(row["disk_size"])
	nDisk := strings.TrimSpace(m.DiskStr)
	if !metricStrEqual(oCPU, nCPU) {
		return true
	}
	if !metricStrEqual(oMem, nMem) {
		return true
	}
	if !metricStrEqual(oDisk, nDisk) {
		return true
	}
	if !metricStrEqual(anyToCompareStr(row["cpu_peak_30"]), strings.TrimSpace(m.CpuPeak30)) {
		return true
	}
	if !metricStrEqual(anyToCompareStr(row["cpu_avg_30"]), strings.TrimSpace(m.CpuAvg30)) {
		return true
	}
	if !metricStrEqual(anyToCompareStr(row["cpu_peak_180"]), strings.TrimSpace(m.CpuPeak180)) {
		return true
	}
	if !metricStrEqual(anyToCompareStr(row["cpu_avg_180"]), strings.TrimSpace(m.CpuAvg180)) {
		return true
	}
	if !metricStrEqual(anyToCompareStr(row["mem_peak_30"]), strings.TrimSpace(m.MemPeak30)) {
		return true
	}
	if !metricStrEqual(anyToCompareStr(row["men_avg_30"]), strings.TrimSpace(m.MenAvg30)) {
		return true
	}
	if !metricStrEqual(anyToCompareStr(row["men_peak_180"]), strings.TrimSpace(m.MenPeak180)) {
		return true
	}
	if !metricStrEqual(anyToCompareStr(row["men_avg_180"]), strings.TrimSpace(m.MenAvg180)) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["middleware_version"])) != strings.TrimSpace(m.MiddlewareVersion) {
		return true
	}
	if strings.TrimSpace(cmdbSecurityGroupStr(row)) != strings.TrimSpace(m.SecurityGroup) {
		return true
	}
	return false
}

func k8sResourceChanged(row map[string]any, clusterUUID string, c k8sCluster, hostIPNew, cloudType, location string) bool {
	u := strings.TrimSpace(clusterUUID)
	if u != "" {
		if strings.TrimSpace(anyToCompareStr(row["uuid"])) != u {
			return true
		}
		if strings.TrimSpace(anyToCompareStr(row["k8s_uuid"])) != u {
			return true
		}
	}
	if strings.TrimSpace(anyToCompareStr(row["host_ip_new"])) != strings.TrimSpace(hostIPNew) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["k8s_version"])) != strings.TrimSpace(c.Version) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["cloud_type"])) != strings.TrimSpace(cloudType) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["location"])) != strings.TrimSpace(location) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["k8s_cluster_name"])) != strings.TrimSpace(c.Name) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["sys_node_name"])) != strings.TrimSpace(c.SysNodeName) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["environment"])) != strings.TrimSpace(c.Environment) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["remarks"])) != strings.TrimSpace(c.Remarks) {
		return true
	}
	return false
}
