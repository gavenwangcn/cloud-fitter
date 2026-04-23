package server

import (
	"context"

	"github.com/golang/glog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbredis"
	"github.com/cloud-fitter/cloud-fitter/internal/server/redis"
)

func (s *Server) ListRedisDetail(ctx context.Context, req *pbredis.ListDetailReq) (*pbredis.ListDetailResp, error) {
	resp, err := redis.ListDetail(ctx, req)
	if err != nil {
		glog.Errorf("ListRedisDetail error %+v", err)
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	return resp, nil
}

func (s *Server) ListRedis(ctx context.Context, req *pbredis.ListReq) (*pbredis.ListResp, error) {
	resp, err := redis.List(ctx, req)
	if err != nil {
		glog.Errorf("ListRedis error %+v", err)
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	return resp, nil
}
