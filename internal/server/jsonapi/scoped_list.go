package jsonapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbcce"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbecs"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbkafka"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbrds"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbredis"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/cloud-fitter/cloud-fitter/internal/clusterenvscratch"
	"github.com/cloud-fitter/cloud-fitter/internal/envtags"
	ccesvc "github.com/cloud-fitter/cloud-fitter/internal/server/cce"
	"github.com/cloud-fitter/cloud-fitter/internal/server/cert"
	"github.com/cloud-fitter/cloud-fitter/internal/server/ecs"
	"github.com/cloud-fitter/cloud-fitter/internal/server/eip"
	"github.com/cloud-fitter/cloud-fitter/internal/server/elb"
	"github.com/cloud-fitter/cloud-fitter/internal/server/kafka"
	"github.com/cloud-fitter/cloud-fitter/internal/server/rds"
	"github.com/cloud-fitter/cloud-fitter/internal/server/redis"
	"github.com/cloud-fitter/cloud-fitter/internal/server/scope"
	"github.com/golang/glog"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type AccountScope struct {
	Provider    int32
	AccountName string
}

// SystemListScope 为按系统名称拉云列表时的解析结果：关联账号 + CMDB system_id（用于标签 system 过滤）。
type SystemListScope struct {
	Accounts []AccountScope
	SystemID string
}

var systemListScopeResolver func(systemName string) (SystemListScope, error)

// SetSystemListScopeResolver 由 main 注入：根据系统名称返回关联云账号与 system_id。
func SetSystemListScopeResolver(resolver func(systemName string) (SystemListScope, error)) {
	systemListScopeResolver = resolver
}

func resolveSystemListScope(systemName string) (SystemListScope, error) {
	if systemListScopeResolver == nil {
		return SystemListScope{}, nil
	}
	return systemListScopeResolver(systemName)
}

func withSystemListCtx(ctx0 context.Context, systemName string) (context.Context, SystemListScope, error) {
	sc, err := resolveSystemListScope(systemName)
	if err != nil {
		return nil, SystemListScope{}, err
	}
	return scope.WithSystemListTagFilter(ctx0, sc.SystemID), sc, nil
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
		resp, cceCtx, err := cceBySystemName(r.Context(), body.SystemName)
		writeCCEListProtoJSON(w, resp, cceCtx, err)
		return
	}
	ctx := clusterenvscratch.Ensure(scope.WithAccountName(r.Context(), body.AccountName))
	resp, err := ccesvc.List(ctx, &pbcce.ListReq{Provider: pbtenant.CloudProvider(body.Provider)})
	writeCCEListProtoJSON(w, resp, ctx, err)
}

// EipByAccount POST /apis/eip/by-account（当前：华为云 EIP）
func EipByAccount(w http.ResponseWriter, r *http.Request) {
	begin := time.Now()
	body, err := decodeListByAccount(r)
	if err != nil {
		glog.Errorf("eip api decode body failed: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	glog.Infof("eip api request provider=%d account=%q system=%q", body.Provider, body.AccountName, body.SystemName)
	if body.SystemName != "" {
		resp, err := eipBySystemName(r.Context(), body.SystemName)
		glog.Infof("eip api response(by-system) system=%q rows=%d err=%v elapsed=%v",
			body.SystemName, len(resp), err, time.Since(begin))
		writeJSON(w, map[string]any{"eips": resp}, err)
		return
	}
	ctx := scope.WithAccountName(r.Context(), body.AccountName)
	resp, err := eip.List(ctx, pbtenant.CloudProvider(body.Provider))
	glog.Infof("eip api response(by-account) provider=%d account=%q rows=%d err=%v elapsed=%v",
		body.Provider, body.AccountName, len(resp), err, time.Since(begin))
	writeJSON(w, map[string]any{"eips": resp}, err)
}

// ElbByAccount POST /apis/elb/by-account（当前：华为云 ELB）
func ElbByAccount(w http.ResponseWriter, r *http.Request) {
	begin := time.Now()
	body, err := decodeListByAccount(r)
	if err != nil {
		glog.Errorf("elb api decode body failed: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	glog.Infof("elb api request provider=%d account=%q system=%q", body.Provider, body.AccountName, body.SystemName)
	if body.SystemName != "" {
		resp, err := elbBySystemName(r.Context(), body.SystemName)
		glog.Infof("elb api response(by-system) system=%q rows=%d err=%v elapsed=%v", body.SystemName, len(resp), err, time.Since(begin))
		writeJSON(w, map[string]any{"elbs": resp}, err)
		return
	}
	ctx := scope.WithAccountName(r.Context(), body.AccountName)
	resp, err := elb.List(ctx, pbtenant.CloudProvider(body.Provider))
	glog.Infof("elb api response(by-account) provider=%d account=%q rows=%d err=%v elapsed=%v",
		body.Provider, body.AccountName, len(resp), err, time.Since(begin))
	writeJSON(w, map[string]any{"elbs": resp}, err)
}

// CertByAccount POST /apis/certificates/by-account（华为云 CCM / SCM 证书列表）
func CertByAccount(w http.ResponseWriter, r *http.Request) {
	begin := time.Now()
	body, err := decodeListByAccount(r)
	if err != nil {
		glog.Errorf("certificates api decode body failed: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	glog.Infof("certificates api request provider=%d account=%q system=%q", body.Provider, body.AccountName, body.SystemName)
	if body.SystemName != "" {
		resp, err := certificatesBySystemName(r.Context(), body.SystemName)
		glog.Infof("certificates api response(by-system) system=%q rows=%d err=%v elapsed=%v",
			body.SystemName, len(resp), err, time.Since(begin))
		writeJSON(w, map[string]any{"certificates": resp}, err)
		return
	}
	ctx := scope.WithAccountName(r.Context(), body.AccountName)
	resp, err := cert.List(ctx, pbtenant.CloudProvider(body.Provider))
	glog.Infof("certificates api response(by-account) provider=%d account=%q rows=%d err=%v elapsed=%v",
		body.Provider, body.AccountName, len(resp), err, time.Since(begin))
	writeJSON(w, map[string]any{"certificates": resp}, err)
}

func ecsBySystemName(ctx0 context.Context, systemName string) (*pbecs.ListResp, error) {
	out := &pbecs.ListResp{}
	baseCtx, sc, err := withSystemListCtx(ctx0, systemName)
	if err != nil {
		return nil, err
	}
	glog.Infof("ecs by-system scope: system_name=%q system_id=%q tag_key=%s linked_accounts=%d",
		systemName, strings.TrimSpace(sc.SystemID), envtags.SystemTagKey(), len(sc.Accounts))
	for _, acc := range sc.Accounts {
		ctx := scope.WithAccountName(baseCtx, acc.AccountName)
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
	baseCtx, sc, err := withSystemListCtx(ctx0, systemName)
	if err != nil {
		return nil, err
	}
	for _, acc := range sc.Accounts {
		ctx := scope.WithAccountName(baseCtx, acc.AccountName)
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
	baseCtx, sc, err := withSystemListCtx(ctx0, systemName)
	if err != nil {
		return nil, err
	}
	for _, acc := range sc.Accounts {
		ctx := scope.WithAccountName(baseCtx, acc.AccountName)
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
	baseCtx, sc, err := withSystemListCtx(ctx0, systemName)
	if err != nil {
		return nil, err
	}
	for _, acc := range sc.Accounts {
		ctx := scope.WithAccountName(baseCtx, acc.AccountName)
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

// cceBySystemName 返回的 scratchCtx 用于 HTTP 写入 envTagValue；非 HTTP 调用方可忽略 scratchCtx。
func cceBySystemName(ctx0 context.Context, systemName string) (*pbcce.ListResp, context.Context, error) {
	out := &pbcce.ListResp{}
	baseCtx, sc, err := withSystemListCtx(ctx0, systemName)
	if err != nil {
		return nil, ctx0, err
	}
	baseCtx = clusterenvscratch.Ensure(baseCtx)
	for _, acc := range sc.Accounts {
		ctx := scope.WithAccountName(baseCtx, acc.AccountName)
		resp, err := ccesvc.List(ctx, &pbcce.ListReq{Provider: pbtenant.CloudProvider(acc.Provider)})
		if err != nil {
			return nil, baseCtx, err
		}
		if resp != nil {
			out.Clusters = append(out.Clusters, resp.Clusters...)
		}
	}
	return out, baseCtx, nil
}

func eipBySystemName(ctx0 context.Context, systemName string) ([]*eip.Instance, error) {
	var out []*eip.Instance
	baseCtx, sc, err := withSystemListCtx(ctx0, systemName)
	if err != nil {
		return nil, err
	}
	for _, acc := range sc.Accounts {
		ctx := scope.WithAccountName(baseCtx, acc.AccountName)
		resp, err := eip.List(ctx, pbtenant.CloudProvider(acc.Provider))
		if err != nil {
			return nil, err
		}
		out = append(out, resp...)
	}
	return out, nil
}

func elbBySystemName(ctx0 context.Context, systemName string) ([]*elb.Instance, error) {
	var out []*elb.Instance
	baseCtx, sc, err := withSystemListCtx(ctx0, systemName)
	if err != nil {
		return nil, err
	}
	for _, acc := range sc.Accounts {
		ctx := scope.WithAccountName(baseCtx, acc.AccountName)
		resp, err := elb.List(ctx, pbtenant.CloudProvider(acc.Provider))
		if err != nil {
			return nil, err
		}
		out = append(out, resp...)
	}
	return out, nil
}

func certificatesBySystemName(ctx0 context.Context, systemName string) ([]*cert.Instance, error) {
	var out []*cert.Instance
	baseCtx, sc, err := withSystemListCtx(ctx0, systemName)
	if err != nil {
		return nil, err
	}
	for _, acc := range sc.Accounts {
		ctx := scope.WithAccountName(baseCtx, acc.AccountName)
		resp, err := cert.List(ctx, pbtenant.CloudProvider(acc.Provider))
		if err != nil {
			return nil, err
		}
		out = append(out, resp...)
	}
	return out, nil
}

// ListEcsBySystemName 供 CMDB 同步等按「系统名称」拉取全量 ECS（与 POST /apis/ecs/by-account 且带 systemName 行为一致）。
func ListEcsBySystemName(ctx context.Context, systemName string) (*pbecs.ListResp, error) {
	return ecsBySystemName(ctx, systemName)
}

// ListRdsBySystemName 同 ListEcsBySystemName，对应 RDS。
func ListRdsBySystemName(ctx context.Context, systemName string) (*pbrds.ListResp, error) {
	return rdsBySystemName(ctx, systemName)
}

// ListRedisBySystemName 同 ListEcsBySystemName，对应 Redis（华为 DCS 等）。
func ListRedisBySystemName(ctx context.Context, systemName string) (*pbredis.ListResp, error) {
	return redisBySystemName(ctx, systemName)
}

// ListKafkaBySystemName 同 ListEcsBySystemName，对应 Kafka / DMS 等。
func ListKafkaBySystemName(ctx context.Context, systemName string) (*pbkafka.ListResp, error) {
	return kafkaBySystemName(ctx, systemName)
}

// ListCceBySystemName 同 ListEcsBySystemName，对应 CCE 集群。
func ListCceBySystemName(ctx context.Context, systemName string) (*pbcce.ListResp, error) {
	resp, _, err := cceBySystemName(ctx, systemName)
	return resp, err
}

// ListEipBySystemName 供 CMDB 同步等按系统名称拉取 EIP（与 POST /apis/eip/by-account 且带 systemName 一致）。
func ListEipBySystemName(ctx context.Context, systemName string) ([]*eip.Instance, error) {
	return eipBySystemName(ctx, systemName)
}

// ListElbBySystemName 供 CMDB 同步等按系统名称拉取 ELB（与 POST /apis/elb/by-account 且带 systemName 一致）。
func ListElbBySystemName(ctx context.Context, systemName string) ([]*elb.Instance, error) {
	return elbBySystemName(ctx, systemName)
}

// writeCCEListProtoJSON 在 protojson 结果上合并 envTagValue（与 ECS/EIP 同源：EnvTagOrNameFallback + ECS 环境键），供 CCE 列表 HTTP 展示。
func writeCCEListProtoJSON(w http.ResponseWriter, resp *pbcce.ListResp, ctx context.Context, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if resp == nil {
		_, _ = w.Write([]byte("{}"))
		return
	}
	envMap := clusterenvscratch.GetMap(ctx)
	b, mErr := protojson.Marshal(resp)
	if mErr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": mErr.Error()})
		return
	}
	var outer map[string]any
	if uErr := json.Unmarshal(b, &outer); uErr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": uErr.Error()})
		return
	}
	if clusters, ok := outer["clusters"].([]any); ok {
		for _, ci := range clusters {
			cm, ok := ci.(map[string]any)
			if !ok {
				continue
			}
			uid, _ := cm["clusterUid"].(string)
			uid = strings.TrimSpace(uid)
			ev := ""
			if envMap != nil && uid != "" {
				if v, ok := envMap[uid]; ok {
					ev = v
				}
			}
			cm["envTagValue"] = ev
		}
	}
	outB, err := json.Marshal(outer)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_, _ = w.Write(outB)
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

func writeJSON(w http.ResponseWriter, data any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(data)
}
