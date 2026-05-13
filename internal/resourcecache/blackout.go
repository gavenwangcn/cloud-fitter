package resourcecache

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
)

var reBlackoutWindow = regexp.MustCompile(`^(\d{1,2}):(\d{2})\s*-\s*(\d{1,2}):(\d{2})$`)

// SnapshotPullBlackoutLocalFromEnv 返回本地时刻 [startMin, endMin) 区间内不拉快照（end 为恢复拉取的首个整分）。
// 时刻均为 24 小时制：默认 01:00–06:00 指凌晨 1 点至 6 点，与 shell 的 date 是否显示 AM/PM 无关。
// 未设置 CLOUD_FITTER_RESOURCE_SNAPSHOT_BLACKOUT_LOCAL 时默认为 01:00–06:00。
// 设为 off、none、false、0、no 则关闭静默窗口。
func SnapshotPullBlackoutLocalFromEnv() (enabled bool, startMin, endMin int) {
	s := strings.TrimSpace(os.Getenv("CLOUD_FITTER_RESOURCE_SNAPSHOT_BLACKOUT_LOCAL"))
	if s == "" {
		return true, 60, 6 * 60 // 01:00–06:00
	}
	low := strings.ToLower(s)
	switch low {
	case "off", "none", "false", "0", "no":
		return false, 0, 0
	}
	m := reBlackoutWindow.FindStringSubmatch(s)
	if m == nil {
		glog.Warningf("resource snapshot blackout: invalid CLOUD_FITTER_RESOURCE_SNAPSHOT_BLACKOUT_LOCAL=%q, using default 01:00-06:00", s)
		return true, 60, 6 * 60
	}
	h1, err1 := strconv.Atoi(m[1])
	mm1, err2 := strconv.Atoi(m[2])
	h2, err3 := strconv.Atoi(m[3])
	mm2, err4 := strconv.Atoi(m[4])
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		glog.Warningf("resource snapshot blackout: parse error for %q, using default 01:00-06:00", s)
		return true, 60, 6 * 60
	}
	startMin = h1*60 + mm1
	endMin = h2*60 + mm2
	if h1 < 0 || h1 > 23 || mm1 < 0 || mm1 > 59 || h2 < 0 || h2 > 23 || mm2 < 0 || mm2 > 59 {
		glog.Warningf("resource snapshot blackout: out-of-range time in %q, using default 01:00-06:00", s)
		return true, 60, 6 * 60
	}
	if startMin >= endMin {
		glog.Warningf("resource snapshot blackout: start must be before end in %q, using default 01:00-06:00", s)
		return true, 60, 6 * 60
	}
	return true, startMin, endMin
}

// InSnapshotPullBlackoutAt 判断 t（已换算到业务时区）是否落在静默窗口内。
func InSnapshotPullBlackoutAt(t time.Time, startMin, endMin int) bool {
	m := t.Hour()*60 + t.Minute()
	return m >= startMin && m < endMin
}

// WaitIfSnapshotPullBlackout 若在静默窗口内则阻塞到窗口结束（按 loc 的日历日计算结束时刻）。
func WaitIfSnapshotPullBlackout(loc *time.Location) {
	if loc == nil {
		loc = time.Local
	}
	enabled, startMin, endMin := SnapshotPullBlackoutLocalFromEnv()
	if !enabled {
		return
	}
	for {
		now := time.Now().In(loc)
		if !InSnapshotPullBlackoutAt(now, startMin, endMin) {
			return
		}
		y, mo, d := now.Date()
		endTime := time.Date(y, mo, d, endMin/60, endMin%60, 0, 0, loc)
		dur := time.Until(endTime)
		if dur <= 0 {
			time.Sleep(time.Second)
			continue
		}
		glog.Infof("resource snapshot: blackout local %02d:%02d-%02d:%02d, sleeping until %s (%v)",
			startMin/60, startMin%60, endMin/60, endMin%60,
			endTime.Format(time.RFC3339), dur)
		time.Sleep(dur + 500*time.Millisecond)
	}
}
