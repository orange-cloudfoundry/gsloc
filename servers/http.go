package servers

import (
	"context"
	"fmt"
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

func (s *HTTPServer) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.ProtoMajor != 2 {
		s.mux.ServeHTTP(writer, request)
		return
	}
	if strings.Contains(request.Header.Get("Content-Type"), "application/grpc") {
		s.grpcServ.ServeHTTP(writer, request)
		return
	}
	s.mux.ServeHTTP(writer, request)
}

func (s *HTTPServer) Run(ctx context.Context) {
	s.mux.Path("/metrics").Handler(promhttp.Handler())
	s.mux.Methods("POST").Path("/hc/{fqdn}/member/{ip}").Handler(s.hcker)

	srvTls := &http.Server{
		Addr:    s.cnf.Listen,
		Handler: s,
	}
	var srvLocal *http.Server
	if s.cnf.ListenLocalPort != 0 {
		localAddr := fmt.Sprintf("127.0.0.1:%d", s.cnf.ListenLocalPort)
		srvLocal = &http.Server{
			Addr:    localAddr,
			Handler: s,
		}
		go func() {
			log.Infof("Starting http server http://%s ...", localAddr)
			err := srvLocal.ListenAndServe()
			if err != nil && err != http.ErrServerClosed {
				log.Fatalf("listen local: %s\n", err.Error())
			}
		}()
	}
	go func() {
		log.Infof("Starting https server https://%s ...", s.cnf.Listen)
		err := srvTls.ListenAndServeTLS(s.cnf.TLSPem.CertPath, s.cnf.TLSPem.PrivateKeyPath)
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err.Error())
		}
	}()
	<-ctx.Done()

	ctxTimeout, cancelFunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFunc()
	log.Info("Graceful shutdown http server ...")
	if srvLocal != nil {
		go func() {
			err := srvLocal.Shutdown(ctxTimeout)
			if err != nil {
				log.Errorf("error when shutdown http: %s", err.Error())
			}
			log.Info("Finished graceful shutdown http server.")
		}()
	}
	err := srvTls.Shutdown(ctxTimeout)
	if err != nil {
		log.Errorf("error when shutdown https: %s", err.Error())
	}
	log.Info("Finished graceful shutdown https server.")
}
