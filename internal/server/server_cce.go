package server

import (
	"context"

	"github.com/golang/glog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbcce"
	"github.com/cloud-fitter/cloud-fitter/internal/server/cce"
)

func (s *Server) ListCceDetail(ctx context.Context, req *pbcce.ListDetailReq) (*pbcce.ListDetailResp, error) {
	resp, err := cce.ListDetail(ctx, req)
	if err != nil {
		glog.Errorf("ListCceDetail error %+v", err)
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	return resp, nil
}

func (s *Server) ListCce(ctx context.Context, req *pbcce.ListReq) (*pbcce.ListResp, error) {
	resp, err := cce.List(ctx, req)
	if err != nil {
		glog.Errorf("ListCce error %+v", err)
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	return resp, nil
}
