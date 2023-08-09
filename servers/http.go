package servers

import (
	"context"
	"github.com/orange-cloudfoundry/gsloc/config"
	"github.com/orange-cloudfoundry/gsloc/healthchecks"
	"google.golang.org/grpc"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

type HTTPServer struct {
	mux      *mux.Router
	cnf      *config.HTTPServerConfig
	hcker    *healthchecks.HcHandler
	grpcServ *grpc.Server
}

func NewHTTPServer(cnf *config.HTTPServerConfig, hcker *healthchecks.HcHandler, grpcServ *grpc.Server) *HTTPServer {
	return &HTTPServer{
		mux:      mux.NewRouter(),
		cnf:      cnf,
		hcker:    hcker,
		grpcServ: grpcServ,
	}
}

func (s *HTTPServer) Run(ctx context.Context) {
	s.mux.Path("/metrics").Handler(promhttp.Handler())
	s.mux.Methods("POST").Path("/hc/{fqdn}/member/{ip}").Handler(s.hcker)

	srv := &http.Server{
		Addr: s.cnf.Listen,
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.ProtoMajor != 2 {
				s.mux.ServeHTTP(writer, request)
				return
			}
			if strings.Contains(request.Header.Get("Content-Type"), "application/grpc") {
				s.grpcServ.ServeHTTP(writer, request)
				return
			}
			s.mux.ServeHTTP(writer, request)
		}),
	}
	log.Infof("Starting http server https://%s ...", s.cnf.Listen)
	go func() {
		err := srv.ListenAndServeTLS(s.cnf.TLSPem.CertPath, s.cnf.TLSPem.PrivateKeyPath)
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err.Error())
		}
	}()
	<-ctx.Done()

	ctxTimeout, cancelFunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFunc()
	log.Info("Graceful shutdown http server ...")
	err := srv.Shutdown(ctxTimeout)
	if err != nil {
		log.Errorf("error when shutdown https: %s", err.Error())
	}
	log.Info("Finished graceful shutdown https server.")
}
