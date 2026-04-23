package jsonapi

import (
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

type listByAccountBody struct {
	Provider    int32  `json:"provider"`
	AccountName string `json:"accountName"`
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
	ctx := scope.WithAccountName(r.Context(), body.AccountName)
	resp, err := ccesvc.List(ctx, &pbcce.ListReq{Provider: pbtenant.CloudProvider(body.Provider)})
	writeProtoJSON(w, resp, err)
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
