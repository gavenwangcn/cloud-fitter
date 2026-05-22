package synclog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// Store 持久化 CMDB 同步批次结果。
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	if db == nil {
		return nil
	}
	return &Store{db: db}
}

// InsertRun 写入一条完整同步批次记录（增量追加，每次同步一条）。
func (st *Store) InsertRun(ctx context.Context, summary RunSummary, runError string) (int64, error) {
	if st == nil || st.db == nil {
		return 0, errors.New("synclog: nil db")
	}
	if summary.ResourceTotals == nil {
		summary.ResourceTotals = map[string]ResourceFailTotal{}
	}
	if summary.Systems == nil {
		summary.Systems = []SystemDetail{}
	}
	totalsJSON, err := json.Marshal(summary.ResourceTotals)
	if err != nil {
		return 0, errors.Wrap(err, "marshal resource totals")
	}
	systemsJSON, err := json.Marshal(summary.Systems)
	if err != nil {
		return 0, errors.Wrap(err, "marshal systems detail")
	}
	finished := summary.FinishedAt
	if finished.IsZero() {
		finished = time.Now()
	}
	started := summary.StartedAt
	if started.IsZero() {
		started = finished
	}
	res, err := st.db.ExecContext(ctx, `
INSERT INTO cmdb_sync_run (
	trigger_type, manual_system_id, manual_system_name,
	started_at, finished_at,
	total_systems, success_systems, failed_systems, skipped_systems,
	status, resource_fail_totals_json, systems_detail_json, error_message
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(summary.TriggerType),
		strings.TrimSpace(summary.ManualSystemID),
		strings.TrimSpace(summary.ManualSystemName),
		started, finished,
		summary.TotalSystems, summary.SuccessSystems, summary.FailedSystems, summary.SkippedSystems,
		summary.Status,
		string(totalsJSON), string(systemsJSON), strings.TrimSpace(runError),
	)
	if err != nil {
		return 0, errors.Wrap(err, "synclog insert run")
	}
	return res.LastInsertId()
}

// StartRun 插入 running 状态记录，返回 run id。
func (st *Store) StartRun(ctx context.Context, trigger TriggerType, manualSystemID, manualSystemName string) (int64, error) {
	if st == nil || st.db == nil {
		return 0, errors.New("synclog: nil db")
	}
	now := time.Now()
	res, err := st.db.ExecContext(ctx, `
INSERT INTO cmdb_sync_run (
	trigger_type, manual_system_id, manual_system_name,
	started_at, status, resource_fail_totals_json, systems_detail_json, error_message
) VALUES (?, ?, ?, ?, 'running', '{}', '[]', '')`,
		string(trigger), strings.TrimSpace(manualSystemID), strings.TrimSpace(manualSystemName), now,
	)
	if err != nil {
		return 0, errors.Wrap(err, "synclog start run")
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, errors.Wrap(err, "synclog last insert id")
	}
	return id, nil
}

// FinishRun 更新批次汇总与明细。runError 为批次级错误（如拉取 CMDB 系统列表失败）。
func (st *Store) FinishRun(ctx context.Context, runID int64, summary RunSummary, runError string) error {
	if st == nil || st.db == nil {
		return errors.New("synclog: nil db")
	}
	if summary.ResourceTotals == nil {
		summary.ResourceTotals = map[string]ResourceFailTotal{}
	}
	if summary.Systems == nil {
		summary.Systems = []SystemDetail{}
	}
	totalsJSON, err := json.Marshal(summary.ResourceTotals)
	if err != nil {
		return errors.Wrap(err, "marshal resource totals")
	}
	systemsJSON, err := json.Marshal(summary.Systems)
	if err != nil {
		return errors.Wrap(err, "marshal systems detail")
	}
	finished := summary.FinishedAt
	if finished.IsZero() {
		finished = time.Now()
	}
	_, err = st.db.ExecContext(ctx, `
UPDATE cmdb_sync_run SET
	finished_at = ?,
	total_systems = ?,
	success_systems = ?,
	failed_systems = ?,
	skipped_systems = ?,
	status = ?,
	resource_fail_totals_json = ?,
	systems_detail_json = ?,
	error_message = ?
WHERE id = ?`,
		finished,
		summary.TotalSystems,
		summary.SuccessSystems,
		summary.FailedSystems,
		summary.SkippedSystems,
		summary.Status,
		string(totalsJSON),
		string(systemsJSON),
		strings.TrimSpace(runError),
		runID,
	)
	return errors.Wrap(err, "synclog finish run")
}

// ListRuns 分页查询同步批次（按 started_at 倒序）。
func (st *Store) ListRuns(ctx context.Context, page, pageSize int) ([]RunSummary, int, error) {
	if st == nil || st.db == nil {
		return nil, 0, errors.New("synclog: nil db")
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	var total int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cmdb_sync_run`).Scan(&total); err != nil {
		return nil, 0, errors.Wrap(err, "count sync runs")
	}
	rows, err := st.db.QueryContext(ctx, `
SELECT id, trigger_type, manual_system_id, manual_system_name,
	started_at, finished_at, total_systems, success_systems, failed_systems, skipped_systems,
	status, resource_fail_totals_json, systems_detail_json
FROM cmdb_sync_run
ORDER BY started_at DESC, id DESC
LIMIT ? OFFSET ?`, pageSize, offset)
	if err != nil {
		return nil, 0, errors.Wrap(err, "list sync runs")
	}
	defer rows.Close()
	var out []RunSummary
	for rows.Next() {
		item, err := scanRunSummary(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

// GetRun 按 id 查询单条同步批次。
func (st *Store) GetRun(ctx context.Context, runID int64) (RunSummary, error) {
	if st == nil || st.db == nil {
		return RunSummary{}, errors.New("synclog: nil db")
	}
	row := st.db.QueryRowContext(ctx, `
SELECT id, trigger_type, manual_system_id, manual_system_name,
	started_at, finished_at, total_systems, success_systems, failed_systems, skipped_systems,
	status, resource_fail_totals_json, systems_detail_json
FROM cmdb_sync_run WHERE id = ?`, runID)
	item, err := scanRunSummary(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RunSummary{}, fmt.Errorf("sync run %d not found", runID)
		}
		return RunSummary{}, err
	}
	return item, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRunSummary(row rowScanner) (RunSummary, error) {
	var item RunSummary
	var trigger, manualID, manualName, status string
	var finished sql.NullTime
	var totalsJSON, systemsJSON string
	if err := row.Scan(
		&item.ID, &trigger, &manualID, &manualName,
		&item.StartedAt, &finished, &item.TotalSystems, &item.SuccessSystems, &item.FailedSystems, &item.SkippedSystems,
		&status, &totalsJSON, &systemsJSON,
	); err != nil {
		return RunSummary{}, errors.Wrap(err, "scan sync run")
	}
	item.TriggerType = TriggerType(trigger)
	item.ManualSystemID = manualID
	item.ManualSystemName = manualName
	item.Status = status
	if finished.Valid {
		item.FinishedAt = finished.Time
	}
	item.ResourceTotals = parseResourceTotalsJSON(totalsJSON)
	item.Systems = []SystemDetail{}
	if strings.TrimSpace(systemsJSON) != "" {
		_ = json.Unmarshal([]byte(systemsJSON), &item.Systems)
	}
	return item, nil
}
