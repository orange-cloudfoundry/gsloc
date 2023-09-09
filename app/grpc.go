package app

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	gslbsvc "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/services/gslb/v1"
	"github.com/orange-cloudfoundry/gsloc/gslb"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

const (
	grpcMaxConcurrentStreams = 1000000
)

// InterceptorLogger adapts logrus logger to interceptor logger.
// This code is simple enough to be copied and not imported.
func InterceptorLogger(l log.FieldLogger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		f := make(log.Fields)
		i := logging.Fields(fields).Iterator()
		for i.Next() {
			k, v := i.At()
			f[k] = v
		}
		l := l.WithFields(f)
		switch lvl {
		case logging.LevelDebug:
			l.Debug(msg)
		case logging.LevelInfo:
			l.Info(msg)
		case logging.LevelWarn:
			l.Warn(msg)
		case logging.LevelError:
			l.Error(msg)
		default:
			panic(fmt.Sprintf("unknown level %v", lvl))
		}
	})
}

func (a *App) makeGrpcOptions() ([]grpc.ServerOption, error) {
	creds, err := credentials.NewServerTLSFromFile(a.cnf.HTTPServer.TLSPem.CertPath, a.cnf.HTTPServer.TLSPem.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("agent: failed to load tls credentials: %v", err)
	}
	grpcOptions := []grpc.ServerOption{
		grpc.Creds(creds),
		grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams),
	}
	grpcOptions = append(grpcOptions, grpc.ChainStreamInterceptor(
		recovery.StreamServerInterceptor(),
		logging.StreamServerInterceptor(InterceptorLogger(log.WithField("component", "grpc_unary_server")), logging.WithLogOnEvents(logging.StartCall, logging.FinishCall)),
		// agent server implements grpc_auth.ServiceAuthFuncOverride interface
		//grpc_auth.StreamServerInterceptor(nullAuth),
		otelgrpc.StreamServerInterceptor(),
	))
	grpcOptions = append(grpcOptions, grpc.ChainUnaryInterceptor(
		recovery.UnaryServerInterceptor(),
		logging.UnaryServerInterceptor(InterceptorLogger(log.WithField("component", "grpc_unary_server")), logging.WithLogOnEvents(logging.StartCall, logging.FinishCall)),
		// agent server implements grpc_auth.ServiceAuthFuncOverride interface
		//grpc_auth.UnaryServerInterceptor(nullAuth),
		otelgrpc.UnaryServerInterceptor(),
	))
	return grpcOptions, nil
}

func (a *App) loadGrpcServer() error {
	if a.onlyServeDns {
		return nil
	}
	grpcOptions, err := a.makeGrpcOptions()
	if err != nil {
		return fmt.Errorf("agent: failed to make grpc options: %v", err)
	}
	grpcServer := grpc.NewServer(grpcOptions...)

	reflection.Register(grpcServer)
	serv, err := gslb.NewServer(a.consulClient)
	if err != nil {
		return fmt.Errorf("agent: failed to create gslb server: %v", err)
	}
	gslbsvc.RegisterGSLBServer(grpcServer, serv)
	grpc_health_v1.RegisterHealthServer(grpcServer, health.NewServer())
	a.grpcServer = grpcServer
	return nil
}
