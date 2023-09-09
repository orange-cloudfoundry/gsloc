package app

import (
	"context"
	"fmt"
	consul "github.com/hashicorp/consul/api"
	"github.com/orange-cloudfoundry/gsloc/config"
	"github.com/orange-cloudfoundry/gsloc/disco"
	"github.com/orange-cloudfoundry/gsloc/geolocs"
	"github.com/orange-cloudfoundry/gsloc/healthchecks"
	"github.com/orange-cloudfoundry/gsloc/lb"
	"github.com/orange-cloudfoundry/gsloc/regs"
	"github.com/orange-cloudfoundry/gsloc/resolvers"
	"github.com/orange-cloudfoundry/gsloc/rets"
	"github.com/orange-cloudfoundry/gsloc/servers"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Closeable interface {
	Close() error
}

type Runnable interface {
	Run(ctx context.Context) error
}

type App struct {
	entry        *log.Entry
	cnf          *config.Config
	consulClient *consul.Client
	ctx          context.Context
	cancelFunc   context.CancelFunc
	consulDisco  *disco.ConsulDiscoverer
	retriever    *rets.Retriever
	gslbHandler  *resolvers.GSLBHandler
	lbFactory    *lb.LBFactory
	geoLoc       *geolocs.GeoLoc
	hcHandler    *healthchecks.HcHandler
	grpcServer   *grpc.Server
	onlyServeDns bool
	noServeDns   bool
}

func NewApp(cnf *config.Config, onlyServeDns, noServeDns bool) (*App, error) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	app := &App{
		entry:        log.WithField("component", "app"),
		cnf:          cnf,
		ctx:          ctx,
		cancelFunc:   cancelFunc,
		onlyServeDns: onlyServeDns,
		noServeDns:   noServeDns,
	}
	err := app.loadConsulClient()
	if err != nil {
		return nil, fmt.Errorf("app loadConsulClient: %w", err)
	}
	err = app.loadConsulDiscoverer()
	if err != nil {
		return nil, fmt.Errorf("app loadConsulDiscoverer: %w", err)
	}
	err = app.loadRetriever()
	if err != nil {
		return nil, fmt.Errorf("app loadRetriever: %w", err)
	}
	err = app.loadGeoLoc()
	if err != nil {
		return nil, fmt.Errorf("app loadGeoLoc: %w", err)
	}
	err = app.loadLbFactory()
	if err != nil {
		return nil, fmt.Errorf("app loadLbFactory: %w", err)
	}
	err = app.loadGSLBHandler()
	if err != nil {
		return nil, fmt.Errorf("app loadGSLBHandler: %w", err)
	}
	err = app.loadHcHandler()
	if err != nil {
		return nil, fmt.Errorf("app loadHcHandler: %w", err)
	}
	err = app.loadGrpcServer()
	if err != nil {
		return nil, fmt.Errorf("app loadGrpcServer: %w", err)
	}
	err = app.register()
	if err != nil {
		return nil, fmt.Errorf("app register: %w", err)
	}
	return app, nil
}

func (a *App) loadConsulClient() error {
	consulConfig := consul.DefaultConfig()
	consulConfig.Address = a.cnf.ConsulConfig.Addr
	consulConfig.Token = a.cnf.ConsulConfig.Token
	consulConfig.Scheme = a.cnf.ConsulConfig.Scheme
	if a.cnf.ConsulConfig.Username != "" && a.cnf.ConsulConfig.Password != "" {
		consulConfig.HttpAuth = &consul.HttpBasicAuth{
			Username: a.cnf.ConsulConfig.Username,
			Password: a.cnf.ConsulConfig.Password,
		}
	}
	consulClient, err := consul.NewClient(consulConfig)
	if err != nil {
		return fmt.Errorf("consul.NewClient: %w", err)
	}
	a.consulClient = consulClient
	return nil
}

func (a *App) loadConsulDiscoverer() error {
	if a.onlyServeDns {
		a.entry.Info("Only serve DNS: no consul discoverer")
		return nil
	}
	consulDisco := disco.NewConsulDiscoverer(
		a.consulClient,
		a.cnf.HealthCheckConfig.HealthcheckAuth,
		a.cnf.DcName,
		a.cnf.HealthCheckConfig.HealthcheckAddress,
	)
	a.consulDisco = consulDisco
	return nil
}

func (a *App) loadRetriever() error {
	retriever := rets.NewRetriever(a.cnf.DcName, 10, time.Duration(*a.cnf.ConsulConfig.ScrapInterval), a.consulClient)
	if a.noServeDns {
		retriever.DisableCatalogPolling()
	}
	a.retriever = retriever
	return nil
}

func (a *App) loadGeoLoc() error {
	if a.noServeDns {
		a.entry.Info("No dns server: no geoloc loaded")
		return nil
	}
	a.geoLoc = geolocs.NewGeoLoc(a.cnf.GeoLoc.DcPositions, a.cnf.GeoLoc.GeoDb.Reader)
	return nil
}

func (a *App) loadLbFactory() error {
	if a.noServeDns {
		a.entry.Info("No dns server: no lb factory loaded")
		return nil
	}
	a.lbFactory = lb.NewLBFactory(a.geoLoc, a.cnf.DNSServer.TrustEdns)
	return nil
}

func (a *App) loadGSLBHandler() error {
	if a.noServeDns {
		a.entry.Info("No dns server: no gslb handler loaded")
		return nil
	}
	_, local4, _ := net.ParseCIDR("127.0.0.1/32") // nolint:errcheck
	_, local6, _ := net.ParseCIDR("::1/128")      // nolint:errcheck
	allowed := []*config.CIDR{
		{
			IpNet: local4,
		},
		{
			IpNet: local6,
		},
	}
	a.gslbHandler = resolvers.NewGSLBHandler(a.lbFactory, append(allowed, a.cnf.DNSServer.AllowedInspect...))
	return nil
}

func (a *App) loadHcHandler() error {
	if a.onlyServeDns {
		a.entry.Info("Only serve DNS: no healthcheck handler")
		return nil
	}
	a.hcHandler = healthchecks.NewHcHandler(a.cnf.HealthCheckConfig)
	return nil
}

func (a *App) register() error {
	if !a.noServeDns {
		regs.DefaultRegCatalog.Register(a.gslbHandler)
	}
	if !a.onlyServeDns {
		regs.DefaultRegKV.Register(a.consulDisco)
		regs.DefaultRegMember.Register(a.hcHandler)
	}
	return nil
}

func (a *App) Config() *config.Config {
	return a.cnf
}

func (a *App) Run() error {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := a.retriever.Run(a.ctx)
		if err != nil {
			log.Panicf("retriever.Run: %v", err)
		}
	}()
	if !a.noServeDns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dnsServer := servers.NewDNSServer(a.cnf.DNSServer, a.gslbHandler)
			dnsServer.Run(a.ctx)
		}()
	}
	if !a.onlyServeDns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			grpcServer := servers.NewHTTPServer(a.cnf.HTTPServer, a.hcHandler, a.grpcServer)
			grpcServer.Run(a.ctx)
		}()
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	a.entry.Infof("Signal (%v) received, gracefully stopping servers ...\n", s)
	a.cancelFunc()

	waitShutdown := make(chan struct{})
	defer close(waitShutdown)

	go func() {
		wg.Wait()
		waitShutdown <- struct{}{}
	}()

	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	select {
	case <-waitShutdown:
		a.entry.Info("All servers stopped gracefully")
		return nil
	case s := <-sig:
		a.entry.Infof("Signal (%v) received consequently, stopping now.", s)
	case <-ticker.C:
		return fmt.Errorf("timeout waiting for handlers to stop")
	}

	return nil
}
