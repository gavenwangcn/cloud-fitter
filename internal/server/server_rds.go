package server

import (
	"context"

	"github.com/golang/glog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbrds"
	"github.com/cloud-fitter/cloud-fitter/internal/server/rds"
)

func (s *Server) ListRdsDetail(ctx context.Context, req *pbrds.ListDetailReq) (*pbrds.ListDetailResp, error) {
	resp, err := rds.ListDetail(ctx, req)
	if err != nil {
		glog.Errorf("ListRdsDetail error %+v", err)
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	return resp, nil
}

func (s *Server) ListRds(ctx context.Context, req *pbrds.ListReq) (*pbrds.ListResp, error) {
	resp, err := rds.List(ctx, req)
	if err != nil {
		glog.Errorf("ListRds error %+v", err)
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	return resp, nil
}

func (s *Server) ListRdsAll(ctx context.Context, req *pbrds.ListAllReq) (*pbrds.ListResp, error) {
	glog.Infof("grpc/http ListRdsAll begin")
	resp, err := rds.ListAll(ctx)
	if err != nil {
		glog.Errorf("ListRdsAll error %+v", err)
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	glog.Infof("grpc/http ListRdsAll ok instances=%d", len(resp.Rdses))
	return resp, nil
}
