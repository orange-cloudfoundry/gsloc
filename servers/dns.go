package servers

import (
	"context"
	"github.com/orange-cloudfoundry/gsloc/config"
	"github.com/orange-cloudfoundry/gsloc/resolvers"
	"time"

	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
)

type DNSServer struct {
	resolver *resolvers.GSLBHandler
	cnf      *config.DNSServerConfig
}

func NewDNSServer(cnf *config.DNSServerConfig, resolver *resolvers.GSLBHandler) *DNSServer {
	return &DNSServer{
		resolver: resolver,
		cnf:      cnf,
	}
}

func runDnsServer(srv *dns.Server) {
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Failed to set udp listener %s\n", err.Error())
	}
}

func (s *DNSServer) Run(ctx context.Context) {
	entry := log.WithField("server", "dns")
	udpServer := &dns.Server{
		Addr:    s.cnf.Listen,
		Net:     "udp",
		Handler: s.resolver,
	}
	tcpServer := &dns.Server{
		Addr:    s.cnf.Listen,
		Net:     "tcp",
		Handler: s.resolver,
	}
	entry.Infof("starting udp and tcp dns server on %s", s.cnf.Listen)
	go runDnsServer(udpServer)
	go runDnsServer(tcpServer)
	<-ctx.Done()
	log.Info("Graceful shutdown dns server ...")
	ctxTimeout, cancelFunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFunc()
	err := udpServer.ShutdownContext(ctxTimeout)
	if err != nil {
		log.Errorf("error when shutdown udp dns server: %s", err.Error())
	}

	ctxTimeout, cancelFunc = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFunc()
	err = tcpServer.ShutdownContext(ctxTimeout)
	if err != nil {
		log.Errorf("error when shutdown udp dns server: %s", err.Error())
	}
	log.Info("Finished graceful shutdown dns server ...")
}
