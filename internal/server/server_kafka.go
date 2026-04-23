package server

import (
	"context"

	"github.com/golang/glog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbkafka"
	"github.com/cloud-fitter/cloud-fitter/internal/server/kafka"
)

func (s *Server) ListKafkaDetail(ctx context.Context, req *pbkafka.ListDetailReq) (*pbkafka.ListDetailResp, error) {
	resp, err := kafka.ListDetail(ctx, req)
	if err != nil {
		glog.Errorf("ListKafkaDetail error %+v", err)
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	return resp, nil
}

func (s *Server) ListKafka(ctx context.Context, req *pbkafka.ListReq) (*pbkafka.ListResp, error) {
	resp, err := kafka.List(ctx, req)
	if err != nil {
		glog.Errorf("ListKafka error %+v", err)
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	return resp, nil
}
