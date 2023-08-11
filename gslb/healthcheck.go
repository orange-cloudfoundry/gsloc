package gslb

import (
	"context"
	gslbsvc "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/services/gslb/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) SetHealthCheck(ctx context.Context, request *gslbsvc.SetHealthCheckRequest) (*emptypb.Empty, error) {
	err := request.ValidateAll()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	signedEntry, err := s.retrieveSignedEntry(request.GetFqdn())
	if err != nil {
		return nil, err
	}

	signedEntry.Healthcheck = request.GetHealthcheck()
	err = s.setSignedEntry(signedEntry)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) GetHealthCheck(ctx context.Context, request *gslbsvc.GetHealthCheckRequest) (*gslbsvc.GetHealthCheckResponse, error) {
	err := request.ValidateAll()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	signedEntry, err := s.retrieveSignedEntry(request.GetFqdn())
	if err != nil {
		return nil, err
	}

	err = s.setSignedEntry(signedEntry)
	if err != nil {
		return nil, err
	}
	return &gslbsvc.GetHealthCheckResponse{
		Healthcheck: signedEntry.Healthcheck,
	}, nil
}
