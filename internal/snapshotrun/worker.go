package snapshotrun

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"time"

	"github.com/golang/glog"

	"github.com/cloud-fitter/cloud-fitter/internal/configstore"
	"github.com/cloud-fitter/cloud-fitter/internal/resourcecache"
)

func snapshotWorkerLocation() *time.Location {
	loc := time.Local
	if v := strings.TrimSpace(os.Getenv("TZ")); v != "" {
		if l, err := time.LoadLocation(v); err == nil {
			loc = l
		} else {
			glog.Warningf("resource snapshot worker: invalid TZ=%q: %v (using Local)", v, err)
		}
	}
	return loc
}

// StartResourceSnapshotWorker 按 resourcecache.SnapshotIntervalFromEnv 周期拉云 API 写入快照表。
// 默认本地 01:00–06:00 不拉取（与 TZ 一致）；可用 CLOUD_FITTER_RESOURCE_SNAPSHOT_BLACKOUT_LOCAL 覆盖或 off 关闭。
func StartResourceSnapshotWorker(store *configstore.Store, db *sql.DB) {
	if store == nil || db == nil || !resourcecache.SnapshotWorkerEnabled() {
		return
	}
	d := resourcecache.SnapshotIntervalFromEnv()
	if d < time.Minute {
		return
	}
	go func() {
		base := context.Background()
		loc := snapshotWorkerLocation()
		glog.Infof("resource snapshot worker: interval=%v TZ=%s first run in 30s", d, loc.String())
		time.Sleep(30 * time.Second)
		resourcecache.WaitIfSnapshotPullBlackout(loc)
		runPullAll(base, store, db)
		t := time.NewTicker(d)
		defer t.Stop()
		for range t.C {
			resourcecache.WaitIfSnapshotPullBlackout(loc)
			runPullAll(base, store, db)
		}
	}()
}

func runPullAll(ctx context.Context, store *configstore.Store, db *sql.DB) {
	systems, err := store.ListSystems()
	if err != nil {
		glog.Errorf("resource snapshot: list systems: %v", err)
		return
	}
	glog.Infof("resource snapshot: pull cycle start systems=%d", len(systems))
	for _, sys := range systems {
		if len(sys.AccountIDs) == 0 {
			continue
		}
		sid := strings.TrimSpace(sys.SystemID)
		name := strings.TrimSpace(sys.Name)
		if sid == "" || name == "" {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 90*time.Minute)
		if err := PullOneSystem(cctx, db, store, name, sid); err != nil {
			glog.Errorf("resource snapshot: system_id=%s name=%q err=%v", sid, name, err)
		}
		cancel()
	}
	glog.Infof("resource snapshot: pull cycle done")
}
