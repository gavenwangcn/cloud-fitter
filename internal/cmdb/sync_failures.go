package cmdb

import (
	"fmt"
	"strings"
)

const maxSyncFailureEntries = 100

// syncFailureEntry 单条资源同步失败明细（含原因，便于落库排查）。
type syncFailureEntry struct {
	ResourceID string
	Reason     string
}

func syncErrMsg(prefix string, err error) string {
	prefix = strings.TrimSpace(prefix)
	if err == nil {
		if prefix != "" {
			return prefix
		}
		return "unknown error"
	}
	if prefix == "" {
		return err.Error()
	}
	return prefix + ": " + err.Error()
}

func (st *componentSyncStats) noteError(resourceID, reason string) {
	st.Errors++
	resourceID = strings.TrimSpace(resourceID)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "unknown error"
	}
	if len(st.Failures) < maxSyncFailureEntries {
		st.Failures = append(st.Failures, syncFailureEntry{
			ResourceID: resourceID,
			Reason:     reason,
		})
	} else if len(st.Failures) == maxSyncFailureEntries {
		st.Failures = append(st.Failures, syncFailureEntry{
			Reason: fmt.Sprintf("… 另有 %d 条失败未展开", st.Errors-maxSyncFailureEntries),
		})
	}
	if resourceID != "" {
		st.FailedIDs = append(st.FailedIDs, resourceID)
	}
}

func mergeComponentStats(a, b componentSyncStats) componentSyncStats {
	a.Added += b.Added
	a.Updated += b.Updated
	a.Skipped += b.Skipped
	a.Deleted += b.Deleted
	a.Errors += b.Errors
	a.FailedIDs = append(a.FailedIDs, b.FailedIDs...)
	a.Failures = append(a.Failures, b.Failures...)
	return a
}
