package resourcecache

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbcce"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbecs"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbkafka"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbrds"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbredis"
	"github.com/cloud-fitter/cloud-fitter/internal/server/eip"
	"github.com/cloud-fitter/cloud-fitter/internal/server/elb"
)

// LoadECS 从快照表还原 ListResp；无行则返回空列表。
func LoadECS(ctx context.Context, db *sql.DB, systemID string) (*pbecs.ListResp, error) {
	out := &pbecs.ListResp{}
	q := "SELECT payload_json FROM " + TableECS + " WHERE system_id = ? ORDER BY resource_key"
	rows, err := db.QueryContext(ctx, q, systemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var m pbecs.EcsInstance
		if err := protojson.Unmarshal([]byte(raw), &m); err != nil {
			return nil, errors.Wrap(err, "unmarshal ecs snapshot row")
		}
		out.Ecses = append(out.Ecses, &m)
	}
	return out, rows.Err()
}

// LoadRDS 从快照表还原 RDS ListResp。
func LoadRDS(ctx context.Context, db *sql.DB, systemID string) (*pbrds.ListResp, error) {
	out := &pbrds.ListResp{}
	rows, err := db.QueryContext(ctx, "SELECT payload_json FROM "+TableRDS+" WHERE system_id = ? ORDER BY resource_key", systemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var m pbrds.RdsInstance
		if err := protojson.Unmarshal([]byte(raw), &m); err != nil {
			return nil, err
		}
		out.Rdses = append(out.Rdses, &m)
	}
	return out, rows.Err()
}

// LoadRedis 从快照表还原 DCS（Redis）ListResp。
func LoadRedis(ctx context.Context, db *sql.DB, systemID string) (*pbredis.ListResp, error) {
	out := &pbredis.ListResp{}
	rows, err := db.QueryContext(ctx, "SELECT payload_json FROM "+TableDCS+" WHERE system_id = ? ORDER BY resource_key", systemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var m pbredis.RedisInstance
		if err := protojson.Unmarshal([]byte(raw), &m); err != nil {
			return nil, err
		}
		out.Redises = append(out.Redises, &m)
	}
	return out, rows.Err()
}

// LoadKafka 从快照表还原 DMS/Kafka ListResp。
func LoadKafka(ctx context.Context, db *sql.DB, systemID string) (*pbkafka.ListResp, error) {
	out := &pbkafka.ListResp{}
	rows, err := db.QueryContext(ctx, "SELECT payload_json FROM "+TableDMS+" WHERE system_id = ? ORDER BY resource_key", systemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var m pbkafka.KafkaInstance
		if err := protojson.Unmarshal([]byte(raw), &m); err != nil {
			return nil, err
		}
		out.Kafkas = append(out.Kafkas, &m)
	}
	return out, rows.Err()
}

// LoadCCE 从快照表还原 CCE ListResp。
func LoadCCE(ctx context.Context, db *sql.DB, systemID string) (*pbcce.ListResp, error) {
	out := &pbcce.ListResp{}
	rows, err := db.QueryContext(ctx, "SELECT payload_json FROM "+TableCCE+" WHERE system_id = ? ORDER BY resource_key", systemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var m pbcce.CceCluster
		if err := protojson.Unmarshal([]byte(raw), &m); err != nil {
			return nil, err
		}
		out.Clusters = append(out.Clusters, &m)
	}
	return out, rows.Err()
}

// LoadEIP 从快照表还原 EIP 列表。
func LoadEIP(ctx context.Context, db *sql.DB, systemID string) ([]*eip.Instance, error) {
	var out []*eip.Instance
	rows, err := db.QueryContext(ctx, "SELECT payload_json FROM "+TableEIP+" WHERE system_id = ? ORDER BY resource_key", systemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var m eip.Instance
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

// LoadELB 从快照表还原 ELB 列表。
func LoadELB(ctx context.Context, db *sql.DB, systemID string) ([]*elb.Instance, error) {
	var out []*elb.Instance
	rows, err := db.QueryContext(ctx, "SELECT payload_json FROM "+TableELB+" WHERE system_id = ? ORDER BY resource_key", systemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var m elb.Instance
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}
