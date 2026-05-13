package resourcecache

import (
	"os"
	"testing"
	"time"
)

func TestInSnapshotPullBlackoutAt(t *testing.T) {
	loc := time.UTC
	// 01:00–06:00
	cases := []struct {
		h, min int
		want   bool
	}{
		{0, 59, false},
		{1, 0, true},
		{3, 30, true},
		{5, 59, true},
		{6, 0, false},
		{12, 0, false},
	}
	for _, tc := range cases {
		tm := time.Date(2026, 5, 13, tc.h, tc.min, 0, 0, loc)
		got := InSnapshotPullBlackoutAt(tm, 60, 6*60)
		if got != tc.want {
			t.Errorf("%02d:%02d got %v want %v", tc.h, tc.min, got, tc.want)
		}
	}
}

func TestSnapshotPullBlackoutLocalFromEnv(t *testing.T) {
	t.Cleanup(func() { _ = os.Unsetenv("CLOUD_FITTER_RESOURCE_SNAPSHOT_BLACKOUT_LOCAL") })

	_ = os.Unsetenv("CLOUD_FITTER_RESOURCE_SNAPSHOT_BLACKOUT_LOCAL")
	en, s, e := SnapshotPullBlackoutLocalFromEnv()
	if !en || s != 60 || e != 360 {
		t.Fatalf("default: got enabled=%v start=%d end=%d", en, s, e)
	}

	_ = os.Setenv("CLOUD_FITTER_RESOURCE_SNAPSHOT_BLACKOUT_LOCAL", "off")
	en, _, _ = SnapshotPullBlackoutLocalFromEnv()
	if en {
		t.Fatal("off should disable")
	}

	_ = os.Setenv("CLOUD_FITTER_RESOURCE_SNAPSHOT_BLACKOUT_LOCAL", "02:30-04:00")
	en, s, e = SnapshotPullBlackoutLocalFromEnv()
	if !en || s != 2*60+30 || e != 4*60 {
		t.Fatalf("custom: got %v %d %d", en, s, e)
	}
}
