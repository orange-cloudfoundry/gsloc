package gslb

import (
	"context"
	"github.com/miekg/dns"
	hcconf "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/healthchecks/v1"
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

	fqdn := dns.CanonicalName(request.GetFqdn())
	signedEntry, err := s.gslocConsul.RetrieveSignedEntry(fqdn)
	if err != nil {
		return nil, err
	}

	err = s.validatePluginHealthCheck(request.GetHealthcheck())
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
	fqdn := dns.CanonicalName(request.GetFqdn())

	signedEntry, err := s.gslocConsul.RetrieveSignedEntry(fqdn)
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

func (s *Server) validatePluginHealthCheck(healthcheck *hcconf.HealthCheck) error {
	if healthcheck == nil {
		return nil
	}
	if healthcheck.GetPluginHealthCheck() == nil {
		return nil
	}
	for _, plugin := range s.hcPlugins {
		if plugin.Name == healthcheck.GetPluginHealthCheck().GetName() {
			return nil
		}
	}
	return status.Errorf(codes.InvalidArgument, "plugin %s not found", healthcheck.GetPluginHealthCheck().GetName())
}
