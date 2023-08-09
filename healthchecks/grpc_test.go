package healthchecks_test

import (
	"crypto/tls"
	hcconf "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/healthchecks/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"log"
	"net"
	"time"
)

var _ = Describe("Grpc", func() {
	var server *grpc.Server
	var healthServer *health.Server
	var lis net.Listener

	Context("Check No Tls", func() {
		BeforeEach(func() {
			var err error
			lis, err = net.Listen("tcp4", "127.0.0.1:0")
			Expect(err).To(BeNil())
			server = grpc.NewServer()
			healthServer = health.NewServer()
			healthpb.RegisterHealthServer(server, healthServer)
			go func() {
				if err := server.Serve(lis); err != nil {
					log.Fatal(err)
				}
			}()
		})
		AfterEach(func() {
			//shut down the httpServer between tests
			server.Stop()
			lis.Close()
		})
		It("should return nil on the most basic test", func() {
			hc := NewGrpcHealthCheck(&hcconf.GrpcHealthCheck{}, 5*time.Second, false, nil)

			err := hc.Check(lis.Addr().String())

			Expect(err).To(BeNil())
		})
		It("should return an error when check fail", func() {
			hc := NewGrpcHealthCheck(&hcconf.GrpcHealthCheck{}, 5*time.Second, false, nil)
			healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)

			err := hc.Check(lis.Addr().String())

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("NOT_SERVING"))
		})
		When("the service is not empty", func() {
			It("should return an error when service not found", func() {
				hc := NewGrpcHealthCheck(&hcconf.GrpcHealthCheck{ServiceName: "test"}, 5*time.Second, false, nil)
				//healthServer.SetServingStatus("test", healthpb.HealthCheckResponse_NOT_SERVING)

				err := hc.Check(lis.Addr().String())

				Expect(err).ToNot(BeNil())
				Expect(err.Error()).To(ContainSubstring("NotFound"))
			})
			It("should return nil when service is found and status serving", func() {
				hc := NewGrpcHealthCheck(&hcconf.GrpcHealthCheck{ServiceName: "test"}, 5*time.Second, false, nil)
				healthServer.SetServingStatus("test", healthpb.HealthCheckResponse_SERVING)

				err := hc.Check(lis.Addr().String())

				Expect(err).To(BeNil())
			})
		})
	})
	Context("Check Tls", func() {
		BeforeEach(func() {
			var err error
			cert, err := tls.X509KeyPair(LocalhostCert, LocalhostKey)
			if err != nil {
				Expect(err).To(BeNil())
			}
			lis, err = tls.Listen("tcp4", "127.0.0.1:0", &tls.Config{
				Certificates: []tls.Certificate{cert},
			})
			Expect(err).To(BeNil())
			server = grpc.NewServer()
			healthServer = health.NewServer()
			healthpb.RegisterHealthServer(server, healthServer)
			go func() {
				if err := server.Serve(lis); err != nil {
					log.Fatal(err)
				}
			}()
		})
		AfterEach(func() {
			//shut down the httpServer between tests
			server.Stop()
			lis.Close()
		})
		It("should return nil on the most basic test", func() {
			hc := NewGrpcHealthCheck(&hcconf.GrpcHealthCheck{}, 5*time.Second, true, &tls.Config{
				InsecureSkipVerify: true,
			})

			err := hc.Check(lis.Addr().String())

			Expect(err).To(BeNil())
		})
	})
})
