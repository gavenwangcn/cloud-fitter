package snapshotrun

import (
	"context"
	"time"

	"github.com/golang/glog"
)

const snapshotCategoryRetryBackoff = 500 * time.Millisecond

// listTwiceIfErr 某一类拉取失败时再执行一次（共 2 次），用于缓解偶发 DNS/网络抖动。
func listTwiceIfErr[T any](ctx context.Context, systemID, systemName, category string, op func(context.Context) (T, error)) (T, error) {
	var zero T
	v, err := op(ctx)
	if err == nil {
		return v, nil
	}
	first := err
	glog.Warningf("resource snapshot: %s first attempt failed system_id=%s name=%q: %v (retry once)", category, systemID, systemName, err)
	t := time.NewTimer(snapshotCategoryRetryBackoff)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-t.C:
	}
	v, err = op(ctx)
	if err != nil {
		return zero, err
	}
	glog.Infof("resource snapshot: %s retry succeeded system_id=%s name=%q (first: %v)", category, systemID, systemName, first)
	return v, nil
}
