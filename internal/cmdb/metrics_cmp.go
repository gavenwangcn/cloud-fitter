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

func serverResourceChanged(row map[string]any, h hostRec) bool {
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
	return false
}

func middlewareResourceChanged(row map[string]any, m mwRec) bool {
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
	if !metricStrEqual(anyToCompareStr(row["mem_peak_30"]), strings.TrimSpace(m.MemPeak30)) {
		return true
	}
	if !metricStrEqual(anyToCompareStr(row["men_avg_30"]), strings.TrimSpace(m.MenAvg30)) {
		return true
	}
	return false
}

func k8sResourceChanged(row map[string]any, hostIPNew, version, cloudType, location string) bool {
	if strings.TrimSpace(anyToCompareStr(row["host_ip_new"])) != strings.TrimSpace(hostIPNew) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["k8s_version"])) != strings.TrimSpace(version) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["cloud_type"])) != strings.TrimSpace(cloudType) {
		return true
	}
	if strings.TrimSpace(anyToCompareStr(row["location"])) != strings.TrimSpace(location) {
		return true
	}
	return false
}
