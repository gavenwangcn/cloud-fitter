package resourcecache

import (
	"os"
	"testing"
)

func TestSnapshotRunOnStartupFromEnv(t *testing.T) {
	t.Cleanup(func() { _ = os.Unsetenv("CLOUD_FITTER_RESOURCE_SNAPSHOT_RUN_ON_STARTUP") })
	_ = os.Unsetenv("CLOUD_FITTER_RESOURCE_SNAPSHOT_RUN_ON_STARTUP")
	if SnapshotRunOnStartupFromEnv() {
		t.Fatal("default should be false")
	}
	_ = os.Setenv("CLOUD_FITTER_RESOURCE_SNAPSHOT_RUN_ON_STARTUP", "true")
	if !SnapshotRunOnStartupFromEnv() {
		t.Fatal("true should enable startup pull")
	}
}
