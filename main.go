package main

import (
	"context"
	"flag"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/pkg/errors"
	"google.golang.org/grpc"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/demo" // Update
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbbilling"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbcce"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbdomain"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbecs"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbkafka"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pboss"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbrds"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbredis"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbstatistic"
	"github.com/cloud-fitter/cloud-fitter/internal/cmdb"
	"github.com/cloud-fitter/cloud-fitter/internal/configstore"
	"github.com/cloud-fitter/cloud-fitter/internal/server"
	"github.com/cloud-fitter/cloud-fitter/internal/server/jsonapi"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

var (
	// command-line options:
	// gRPC server endpoint
	grpcServerEndpoint = flag.String("grpc-server-endpoint", ":9091", "gRPC server endpoint")
	// CMDB 同步（与 cmdb-sync 写入逻辑一致，云数据来自本进程 jsonapi 同源 List）。留空则不从命令行启用；环境变量见 cmdb.CMDBConfigFromEnv
	cmdbBaseURL  = flag.String("cmdb-base-url", "", "CMDB API base URL, enables daily CMDB sync at 02:00 if key/secret set")
	cmdbKey      = flag.String("cmdb-key", "", "CMDB API _key (overrides CLOUD_FITTER_CMDB_KEY if set)")
	cmdbSecret   = flag.String("cmdb-secret", "", "CMDB API signing secret (overrides CLOUD_FITTER_CMDB_SECRET if set)")
)

func run(store *configstore.Store, cmdbSyncer *cmdb.Syncer) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register gRPC server endpoint
	// Note: Make sure the gRPC server is running properly and accessible
	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithInsecure()}

	if err := demo.RegisterDemoServiceHandlerFromEndpoint(ctx, mux, *grpcServerEndpoint, opts); err != nil {
		return errors.Wrap(err, "RegisterDemoServiceHandlerFromEndpoint error")
	} else if err = pbecs.RegisterEcsServiceHandlerFromEndpoint(ctx, mux, *grpcServerEndpoint, opts); err != nil {
		return errors.Wrap(err, "RegisterEcsServiceHandlerFromEndpoint error")
	} else if err = pbstatistic.RegisterStatisticServiceHandlerFromEndpoint(ctx, mux, *grpcServerEndpoint, opts); err != nil {
		return errors.Wrap(err, "RegisterStatisticServiceHandlerFromEndpoint error")
	} else if err = pbrds.RegisterRdsServiceHandlerFromEndpoint(ctx, mux, *grpcServerEndpoint, opts); err != nil {
		return errors.Wrap(err, "RegisterRdsServiceHandlerFromEndpoint error")
	} else if err = pbredis.RegisterRedisServiceHandlerFromEndpoint(ctx, mux, *grpcServerEndpoint, opts); err != nil {
		return errors.Wrap(err, "RegisterRedisServiceHandlerFromEndpoint error")
	} else if err = pbdomain.RegisterDomainServiceHandlerFromEndpoint(ctx, mux, *grpcServerEndpoint, opts); err != nil {
		return errors.Wrap(err, "RegisterDomainServiceHandlerFromEndpoint error")
	} else if err = pboss.RegisterOssServiceHandlerFromEndpoint(ctx, mux, *grpcServerEndpoint, opts); err != nil {
		return errors.Wrap(err, "RegisterOssServiceHandlerFromEndpoint error")
	} else if err = pbkafka.RegisterKafkaServiceHandlerFromEndpoint(ctx, mux, *grpcServerEndpoint, opts); err != nil {
		return errors.Wrap(err, "RegisterKafkaServiceHandlerFromEndpoint error")
	} else if err = pbcce.RegisterCceServiceHandlerFromEndpoint(ctx, mux, *grpcServerEndpoint, opts); err != nil {
		return errors.Wrap(err, "RegisterCceServiceHandlerFromEndpoint error")
	} else if err = pbbilling.RegisterBillingServiceHandlerFromEndpoint(ctx, mux, *grpcServerEndpoint, opts); err != nil {
		return errors.Wrap(err, "RegisterBillingServiceHandlerFromEndpoint error")
	}

	reloadTenants := func() error {
		cfg, err := store.ToCloudConfigs()
		if err != nil {
			return err
		}
		return tenanter.ReloadFromConfigs(cfg)
	}
	configHandler := configstore.HTTPHandler(store, reloadTenants)
	systemHandler := configstore.SystemHTTPHandler(store)
	jsonapi.SetSystemAccountResolver(func(systemName string) ([]jsonapi.AccountScope, error) {
		rows, err := store.AccountsBySystemName(systemName)
		if err != nil {
			return nil, err
		}
		out := make([]jsonapi.AccountScope, 0, len(rows))
		for _, r := range rows {
			out = append(out, jsonapi.AccountScope{
				Provider:    r.Provider,
				AccountName: r.Name,
			})
		}
		return out, nil
	})

	// Start HTTP server (grpc-gateway JSON API，与容器/compose 暴露端口 9090 一致)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		glog.Infof("http %s %s", r.Method, r.URL.RequestURI())
		switch {
		case r.URL.Path == "/apis/configs" && (r.Method == http.MethodGet || r.Method == http.MethodPost || r.Method == http.MethodDelete):
			configHandler.ServeHTTP(w, r)
		case r.URL.Path == "/apis/systems" && (r.Method == http.MethodGet || r.Method == http.MethodPost || r.Method == http.MethodPut):
			systemHandler.ServeHTTP(w, r)
		case r.URL.Path == "/apis/cmdb/sync" && r.Method == http.MethodPost:
			cmdb.SyncHTTPHandler(cmdbSyncer).ServeHTTP(w, r)
		case r.URL.Path == "/apis/ecs/by-account" && r.Method == http.MethodPost:
			jsonapi.EcsByAccount(w, r)
		case r.URL.Path == "/apis/rds/by-account" && r.Method == http.MethodPost:
			jsonapi.RdsByAccount(w, r)
		case r.URL.Path == "/apis/redis/by-account" && r.Method == http.MethodPost:
			jsonapi.RedisByAccount(w, r)
		case r.URL.Path == "/apis/kafka/by-account" && r.Method == http.MethodPost:
			jsonapi.KafkaByAccount(w, r)
		case r.URL.Path == "/apis/cce/by-account" && r.Method == http.MethodPost:
			jsonapi.CceByAccount(w, r)
		case r.URL.Path == "/apis/billing/by-account" && r.Method == http.MethodPost:
			jsonapi.BillingSummaryByAccount(w, r)
		default:
			mux.ServeHTTP(w, r)
		}
		glog.Infof("http %s %s done in %v", r.Method, r.URL.RequestURI(), time.Since(start))
	})
	return http.ListenAndServe(":9090", h)
}

func main() {
	var configFile string
	var sqlitePath string
	flag.StringVar(&configFile, "conf", "config.yaml", "config file path")
	flag.StringVar(&sqlitePath, "sqlitedb", "cloud-fitter.db", "sqlite database path (当未使用 MySQL 时)")
	flag.Parse()
	defer glog.Flush()

	var store *configstore.Store
	var err error
	if configstore.UseMySQLFromEnv() {
		dsn, dsnErr := configstore.MySQLDSNFromEnv()
		if dsnErr != nil {
			glog.Fatalf("mysql dsn: %v", dsnErr)
		}
		store, err = configstore.OpenMySQL(dsn)
		if err != nil {
			glog.Fatalf("open mysql: %v", err)
		}
	} else {
		store, err = configstore.Open(sqlitePath)
		if err != nil {
			glog.Fatalf("open sqlite %s: %v", sqlitePath, err)
		}
	}
	defer store.Close()

	n, err := store.Count()
	if err != nil {
		glog.Fatalf("config store count: %v", err)
	}
	if n > 0 {
		cfg, err := store.ToCloudConfigs()
		if err != nil {
			glog.Fatalf("ToCloudConfigs: %v", err)
		}
		if err := tenanter.ReloadFromConfigs(cfg); err != nil {
			glog.Fatalf("ReloadFromConfigs: %v", err)
		}
		if configstore.UseMySQLFromEnv() {
			glog.Infof("loaded %d cloud config row(s) from MySQL", n)
		} else {
			glog.Infof("loaded %d cloud config row(s) from sqlite %s", n, sqlitePath)
		}
	} else {
		if err := tenanter.LoadCloudConfigsFromFile(configFile); err != nil {
			if !errors.Is(err, tenanter.ErrLoadTenanterFileEmpty) {
				glog.Fatalf("LoadCloudConfigsFromFile error %+v", err)
			}
			glog.Warningf("LoadCloudConfigsFromFile empty file path %s", configFile)
		}
		if configstore.UseMySQLFromEnv() {
			glog.Infof("MySQL 中无云账号行: 若存在则尝试从 yaml 加载")
		} else {
			glog.Infof("sqlite empty: load tenant from yaml if present")
		}
	}

	go func() {
		lis, err := net.Listen("tcp", ":9091")
		if err != nil {
			glog.Fatalf("failed to listen: %v", err)
		}

		s := grpc.NewServer()
		demo.RegisterDemoServiceServer(s, &server.Server{})
		pbecs.RegisterEcsServiceServer(s, &server.Server{})
		pbstatistic.RegisterStatisticServiceServer(s, &server.Server{})
		pbrds.RegisterRdsServiceServer(s, &server.Server{})
		pbredis.RegisterRedisServiceServer(s, &server.Server{})
		pbdomain.RegisterDomainServiceServer(s, &server.Server{})
		pboss.RegisterOssServiceServer(s, &server.Server{})
		pbkafka.RegisterKafkaServiceServer(s, &server.Server{})
		pbcce.RegisterCceServiceServer(s, &server.Server{})
		pbbilling.RegisterBillingServiceServer(s, &server.Server{})

		if err = s.Serve(lis); err != nil {
			glog.Fatalf("failed to serve: %v", err)
		}
	}()

	var cmdbSyncer *cmdb.Syncer
	{
		base, key, sec, _ := cmdb.CMDBConfigFromEnv()
		if b := strings.TrimSpace(*cmdbBaseURL); b != "" {
			base = b
		}
		if v := strings.TrimSpace(*cmdbKey); v != "" {
			key = v
		}
		if v := strings.TrimSpace(*cmdbSecret); v != "" {
			sec = v
		}
		if base != "" && key != "" && sec != "" {
			cmdbSyncer = &cmdb.Syncer{Client: cmdb.NewClient(base, key, sec), Store: store}
			glog.Infof("CMDB client configured baseURL=%q", base)
		}
	}
	if cmdbSyncer != nil {
		cmdbSyncer.StartDailyAt(2, 0)
		glog.Infof("CMDB daily sync enabled at 02:00 (local)")
	}

	if err := run(store, cmdbSyncer); err != nil {
		glog.Fatal(err)
	}
}
