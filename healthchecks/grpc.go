package healthchecks

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	hcconf "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/healthchecks/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"time"
)

type GrpcHealthCheckOpt struct { // An optional service name parameter which will be sent to gRPC service in
	// `grpc.health.v1.HealthCheckRequest
	// <https://github.com/grpc/grpc/blob/master/src/proto/grpc/health/v1/health.proto#L20>`_.
	// message. See `gRPC health-checking overview
	// <https://github.com/grpc/grpc/blob/master/doc/health-checking.md>`_ for more information.
	ServiceName string `json:"service_name,omitempty"`
	// The value of the :authority header in the gRPC health check request. If
	// left empty (default value), the name of the cluster this health check is associated
	// with will be used. The authority header can be customized for a specific endpoint by setting
	// the HealthCheckConfig.hostname field.
	Authority string `json:"authority,omitempty"`
}

type GrpcHealthCheck struct {
	conf       *hcconf.GrpcHealthCheck
	tlsEnabled bool
	timeout    time.Duration
	tlsConf    *tls.Config
}

func NewGrpcHealthCheck(conf *hcconf.GrpcHealthCheck, timeout time.Duration, tlsEnabled bool, tlsConf *tls.Config) *GrpcHealthCheck {
	return &GrpcHealthCheck{
		conf:       conf,
		tlsEnabled: tlsEnabled,
		timeout:    timeout,
		tlsConf:    tlsConf,
	}
}

func (h *GrpcHealthCheck) Check(host string) error {
	conn, err := h.makeGrpcConn(host)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()
	client := healthpb.NewHealthClient(conn)
	resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{
		Service: h.conf.ServiceName,
	})
	if err != nil {
		if stat, ok := status.FromError(err); ok {
			switch stat.Code() {
			case codes.Unimplemented:
				return fmt.Errorf("gRPC server does not implement the health protocol: %w", err)
			case codes.DeadlineExceeded:
				return fmt.Errorf("gRPC health check timeout: %w", err)
			}
		}

		return fmt.Errorf("gRPC health check failed: %w", err)
	}

	if resp.Status != healthpb.HealthCheckResponse_SERVING {
		return fmt.Errorf("received gRPC status code: %v", resp.Status)
	}
	return nil
}

func (h *GrpcHealthCheck) makeGrpcConn(host string) (*grpc.ClientConn, error) {

	var opts []grpc.DialOption
	if !h.tlsEnabled {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(h.tlsConf)))
	}
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, host, opts...)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("fail to connect to %s within %s: %w", host, h.timeout, err)
		}
		return nil, fmt.Errorf("fail to connect to %s: %w", host, err)
	}
	return conn, nil
}
