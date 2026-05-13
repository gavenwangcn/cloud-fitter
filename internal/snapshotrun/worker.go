package snapshotrun

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/golang/glog"

	"github.com/cloud-fitter/cloud-fitter/internal/configstore"
	"github.com/cloud-fitter/cloud-fitter/internal/resourcecache"
)

// StartResourceSnapshotWorker 按 resourcecache.SnapshotIntervalFromEnv 周期拉云 API 写入快照表。
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
		glog.Infof("resource snapshot worker: interval=%v first run in 30s", d)
		time.Sleep(30 * time.Second)
		runPullAll(base, store, db)
		t := time.NewTicker(d)
		defer t.Stop()
		for range t.C {
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
