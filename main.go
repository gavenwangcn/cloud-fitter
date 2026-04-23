package main

import (
	"context"
	"flag"
	"net"
	"net/http"
	"time"

	"github.com/golang/glog"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/pkg/errors"
	"google.golang.org/grpc"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/demo" // Update
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbbilling"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbdomain"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbecs"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbkafka"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pboss"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbrds"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbredis"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbstatistic"
	"github.com/cloud-fitter/cloud-fitter/internal/configstore"
	"github.com/cloud-fitter/cloud-fitter/internal/server"
	"github.com/cloud-fitter/cloud-fitter/internal/server/jsonapi"
	"github.com/cloud-fitter/cloud-fitter/internal/tenanter"
)

var (
	// command-line options:
	// gRPC server endpoint
	grpcServerEndpoint = flag.String("grpc-server-endpoint", ":9091", "gRPC server endpoint")
)

func run(store *configstore.Store) error {
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

	// Start HTTP server (grpc-gateway JSON API，与容器/compose 暴露端口 9090 一致)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		glog.Infof("http %s %s", r.Method, r.URL.RequestURI())
		switch {
		case r.URL.Path == "/apis/configs" && (r.Method == http.MethodGet || r.Method == http.MethodPost):
			configHandler.ServeHTTP(w, r)
		case r.URL.Path == "/apis/ecs/by-account" && r.Method == http.MethodPost:
			jsonapi.EcsByAccount(w, r)
		case r.URL.Path == "/apis/rds/by-account" && r.Method == http.MethodPost:
			jsonapi.RdsByAccount(w, r)
		case r.URL.Path == "/apis/redis/by-account" && r.Method == http.MethodPost:
			jsonapi.RedisByAccount(w, r)
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
	flag.StringVar(&sqlitePath, "sqlitedb", "cloud-fitter.db", "sqlite database path for cloud account configs")
	flag.Parse()
	defer glog.Flush()

	store, err := configstore.Open(sqlitePath)
	if err != nil {
		glog.Fatalf("open sqlite %s: %v", sqlitePath, err)
	}
	defer store.Close()

	n, err := store.Count()
	if err != nil {
		glog.Fatalf("sqlite count: %v", err)
	}
	if n > 0 {
		cfg, err := store.ToCloudConfigs()
		if err != nil {
			glog.Fatalf("sqlite ToCloudConfigs: %v", err)
		}
		if err := tenanter.ReloadFromConfigs(cfg); err != nil {
			glog.Fatalf("ReloadFromConfigs: %v", err)
		}
		glog.Infof("loaded %d cloud config row(s) from sqlite %s", n, sqlitePath)
	} else {
		if err := tenanter.LoadCloudConfigsFromFile(configFile); err != nil {
			if !errors.Is(err, tenanter.ErrLoadTenanterFileEmpty) {
				glog.Fatalf("LoadCloudConfigsFromFile error %+v", err)
			}
			glog.Warningf("LoadCloudConfigsFromFile empty file path %s", configFile)
		}
		glog.Infof("sqlite empty: load tenant from yaml if present")
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
		pbbilling.RegisterBillingServiceServer(s, &server.Server{})

		if err = s.Serve(lis); err != nil {
			glog.Fatalf("failed to serve: %v", err)
		}
	}()

	if err := run(store); err != nil {
		glog.Fatal(err)
	}
}
