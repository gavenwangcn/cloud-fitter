package jsonapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbcce"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbecs"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbkafka"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbredis"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbrds"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	ccesvc "github.com/cloud-fitter/cloud-fitter/internal/server/cce"
	"github.com/cloud-fitter/cloud-fitter/internal/server/ecs"
	"github.com/cloud-fitter/cloud-fitter/internal/server/kafka"
	"github.com/cloud-fitter/cloud-fitter/internal/server/rds"
	"github.com/cloud-fitter/cloud-fitter/internal/server/redis"
	"github.com/cloud-fitter/cloud-fitter/internal/server/scope"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type AccountScope struct {
	Provider    int32
	AccountName string
}

var systemAccountResolver func(systemName string) ([]AccountScope, error)

func SetSystemAccountResolver(resolver func(systemName string) ([]AccountScope, error)) {
	systemAccountResolver = resolver
}

type listByAccountBody struct {
	Provider    int32  `json:"provider"`
	AccountName string `json:"accountName"`
	SystemName  string `json:"systemName"`
}

func decodeListByAccount(r *http.Request) (listByAccountBody, error) {
	var body listByAccountBody
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return body, err
	}
	if err := json.Unmarshal(b, &body); err != nil {
		return body, err
	}
	return body, nil
}

// EcsByAccount POST /apis/ecs/by-account
func EcsByAccount(w http.ResponseWriter, r *http.Request) {
	body, err := decodeListByAccount(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.SystemName != "" {
		resp, err := ecsBySystemName(r.Context(), body.SystemName)
		writeProtoJSON(w, resp, err)
		return
	}
	ctx := scope.WithAccountName(r.Context(), body.AccountName)
	resp, err := ecs.List(ctx, &pbecs.ListReq{Provider: pbtenant.CloudProvider(body.Provider)})
	writeProtoJSON(w, resp, err)
}

// RdsByAccount POST /apis/rds/by-account
func RdsByAccount(w http.ResponseWriter, r *http.Request) {
	body, err := decodeListByAccount(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.SystemName != "" {
		resp, err := rdsBySystemName(r.Context(), body.SystemName)
		writeProtoJSON(w, resp, err)
		return
	}
	ctx := scope.WithAccountName(r.Context(), body.AccountName)
	resp, err := rds.List(ctx, &pbrds.ListReq{Provider: pbtenant.CloudProvider(body.Provider)})
	writeProtoJSON(w, resp, err)
}

// RedisByAccount POST /apis/redis/by-account
func RedisByAccount(w http.ResponseWriter, r *http.Request) {
	body, err := decodeListByAccount(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.SystemName != "" {
		resp, err := redisBySystemName(r.Context(), body.SystemName)
		writeProtoJSON(w, resp, err)
		return
	}
	ctx := scope.WithAccountName(r.Context(), body.AccountName)
	resp, err := redis.List(ctx, &pbredis.ListReq{Provider: pbtenant.CloudProvider(body.Provider)})
	writeProtoJSON(w, resp, err)
}

// KafkaByAccount POST /apis/kafka/by-account（DMS / Kafka 实例）
func KafkaByAccount(w http.ResponseWriter, r *http.Request) {
	body, err := decodeListByAccount(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.SystemName != "" {
		resp, err := kafkaBySystemName(r.Context(), body.SystemName)
		writeProtoJSON(w, resp, err)
		return
	}
	ctx := scope.WithAccountName(r.Context(), body.AccountName)
	resp, err := kafka.List(ctx, &pbkafka.ListReq{Provider: pbtenant.CloudProvider(body.Provider)})
	writeProtoJSON(w, resp, err)
}

// CceByAccount POST /apis/cce/by-account（当前：华为云 CCE 集群）
func CceByAccount(w http.ResponseWriter, r *http.Request) {
	body, err := decodeListByAccount(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.SystemName != "" {
		resp, err := cceBySystemName(r.Context(), body.SystemName)
		writeProtoJSON(w, resp, err)
		return
	}
	ctx := scope.WithAccountName(r.Context(), body.AccountName)
	resp, err := ccesvc.List(ctx, &pbcce.ListReq{Provider: pbtenant.CloudProvider(body.Provider)})
	writeProtoJSON(w, resp, err)
}

func resolveSystemAccounts(systemName string) ([]AccountScope, error) {
	if systemAccountResolver == nil {
		return nil, nil
	}
	return systemAccountResolver(systemName)
}

func ecsBySystemName(ctx0 context.Context, systemName string) (*pbecs.ListResp, error) {
	out := &pbecs.ListResp{}
	accounts, err := resolveSystemAccounts(systemName)
	if err != nil {
		return nil, err
	}
	for _, acc := range accounts {
		ctx := scope.WithAccountName(ctx0, acc.AccountName)
		resp, err := ecs.List(ctx, &pbecs.ListReq{Provider: pbtenant.CloudProvider(acc.Provider)})
		if err != nil {
			return nil, err
		}
		if resp != nil {
			out.Ecses = append(out.Ecses, resp.Ecses...)
		}
	}
	return out, nil
}

func rdsBySystemName(ctx0 context.Context, systemName string) (*pbrds.ListResp, error) {
	out := &pbrds.ListResp{}
	accounts, err := resolveSystemAccounts(systemName)
	if err != nil {
		return nil, err
	}
	for _, acc := range accounts {
		ctx := scope.WithAccountName(ctx0, acc.AccountName)
		resp, err := rds.List(ctx, &pbrds.ListReq{Provider: pbtenant.CloudProvider(acc.Provider)})
		if err != nil {
			return nil, err
		}
		if resp != nil {
			out.Rdses = append(out.Rdses, resp.Rdses...)
		}
	}
	return out, nil
}

func redisBySystemName(ctx0 context.Context, systemName string) (*pbredis.ListResp, error) {
	out := &pbredis.ListResp{}
	accounts, err := resolveSystemAccounts(systemName)
	if err != nil {
		return nil, err
	}
	for _, acc := range accounts {
		ctx := scope.WithAccountName(ctx0, acc.AccountName)
		resp, err := redis.List(ctx, &pbredis.ListReq{Provider: pbtenant.CloudProvider(acc.Provider)})
		if err != nil {
			return nil, err
		}
		if resp != nil {
			out.Redises = append(out.Redises, resp.Redises...)
		}
	}
	return out, nil
}

func kafkaBySystemName(ctx0 context.Context, systemName string) (*pbkafka.ListResp, error) {
	out := &pbkafka.ListResp{}
	accounts, err := resolveSystemAccounts(systemName)
	if err != nil {
		return nil, err
	}
	for _, acc := range accounts {
		ctx := scope.WithAccountName(ctx0, acc.AccountName)
		resp, err := kafka.List(ctx, &pbkafka.ListReq{Provider: pbtenant.CloudProvider(acc.Provider)})
		if err != nil {
			return nil, err
		}
		if resp != nil {
			out.Kafkas = append(out.Kafkas, resp.Kafkas...)
		}
	}
	return out, nil
}

func cceBySystemName(ctx0 context.Context, systemName string) (*pbcce.ListResp, error) {
	out := &pbcce.ListResp{}
	accounts, err := resolveSystemAccounts(systemName)
	if err != nil {
		return nil, err
	}
	for _, acc := range accounts {
		ctx := scope.WithAccountName(ctx0, acc.AccountName)
		resp, err := ccesvc.List(ctx, &pbcce.ListReq{Provider: pbtenant.CloudProvider(acc.Provider)})
		if err != nil {
			return nil, err
		}
		if resp != nil {
			out.Clusters = append(out.Clusters, resp.Clusters...)
		}
	}
	return out, nil
}

func writeProtoJSON(w http.ResponseWriter, msg proto.Message, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	b, err := protojson.Marshal(msg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_, _ = w.Write(b)
}
