package resourcecache

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbbilling"
)

// BillingAccountRow 单云账号在某一账期下的账单汇总快照。
type BillingAccountRow struct {
	Provider    int32
	AccountName string
	PayloadJSON string
}

// ReplaceBillingSnapshot 删除该系统该账期下全部账单快照后插入新行；rows 为空则不删库（与「接口全失败不写」一致）。
func ReplaceBillingSnapshot(ctx context.Context, db *sql.DB, systemID, systemName, billingMonth string, rows []BillingAccountRow) error {
	if db == nil {
		return errors.New("resourcecache: nil db")
	}
	if len(rows) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "DELETE FROM "+TableBilling+" WHERE system_id = ? AND billing_month = ?", systemID, billingMonth); err != nil {
		return errors.Wrap(err, "delete cloud_snap_billing")
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	ins := "INSERT INTO " + TableBilling + " (system_id, system_name, billing_month, provider, account_name, payload_json, updated_at) VALUES (?,?,?,?,?,?,?)"
	for _, r := range rows {
		if _, err := tx.ExecContext(ctx, ins, systemID, systemName, billingMonth, r.Provider, r.AccountName, r.PayloadJSON, now); err != nil {
			return errors.Wrapf(err, "insert cloud_snap_billing account=%s", r.AccountName)
		}
	}
	return tx.Commit()
}

// LoadBilling 读取单账号账单汇总快照；无行返回 (nil, nil)。
func LoadBilling(ctx context.Context, db *sql.DB, systemID, billingMonth string, provider int32, accountName string) (*pbbilling.ListBillingSummaryResp, error) {
	if db == nil {
		return nil, errors.New("resourcecache LoadBilling: nil db")
	}
	var raw string
	err := db.QueryRowContext(ctx,
		"SELECT payload_json FROM "+TableBilling+" WHERE system_id = ? AND billing_month = ? AND provider = ? AND account_name = ?",
		systemID, billingMonth, provider, accountName,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out pbbilling.ListBillingSummaryResp
	if err := protojson.Unmarshal([]byte(raw), &out); err != nil {
		return nil, errors.Wrap(err, "unmarshal billing snapshot")
	}
	return &out, nil
}
