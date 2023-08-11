package healthchecks_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	hcconf "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/healthchecks/v1"
	. "github.com/orange-cloudfoundry/gsloc/healthchecks"
	"github.com/orange-cloudfoundry/gsloc/testhelpers"
	"net"
	"sync/atomic"

	"time"
)

var _ = Describe("Tcp", func() {
	var lis net.Listener
	Context("Check No Tls", func() {
		BeforeEach(func() {
			var err error
			lis, err = net.Listen("tcp4", "127.0.0.1:0")
			Expect(err).To(BeNil())

		})
		AfterEach(func() {
			lis.Close()
		})
		It("should return nil on the most basic test", func() {
			hc := NewTcpHealthCheck(&hcconf.TcpHealthCheck{}, 5*time.Second, false, nil)

			err := hc.Check(lis.Addr().String())

			Expect(err).To(BeNil())
		})
		It("should return an error when could not connect", func() {
			lis.Close()
			hc := NewTcpHealthCheck(&hcconf.TcpHealthCheck{}, 5*time.Second, false, nil)

			err := hc.Check(lis.Addr().String())

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("connection refused"))
		})
		It("should pass send payload to tcp", func() {
			var ops int64
			setConnHandlerListener(lis, func(conn net.Conn) {
				defer GinkgoRecover()
				buf := make([]byte, 4)
				reqLen, err := conn.Read(buf)
				Expect(err).To(BeNil())
				Expect(string(buf[:reqLen])).To(Equal("test"))
				atomic.AddInt64(&ops, 1)
			})
			hc := NewTcpHealthCheck(&hcconf.TcpHealthCheck{
				Send: &hcconf.HealthCheckPayload{
					Payload: &hcconf.HealthCheckPayload_Text{
						Text: "test",
					},
				},
			}, 5*time.Second, false, nil)

			err := hc.Check(lis.Addr().String())

			Expect(err).To(BeNil())
			testhelpers.EventuallyAtomic(&ops).Should(Equal(1))
		})
		It("should return nil if found payload", func() {
			var ops int64
			setConnHandlerListener(lis, func(conn net.Conn) {
				defer GinkgoRecover()
				_, err := conn.Write([]byte("test"))
				Expect(err).To(BeNil())
				_, err = conn.Write([]byte("test2"))
				Expect(err).To(BeNil())
				atomic.AddInt64(&ops, 1)
			})
			hc := NewTcpHealthCheck(&hcconf.TcpHealthCheck{
				Receive: []*hcconf.HealthCheckPayload{
					{
						Payload: &hcconf.HealthCheckPayload_Text{
							Text: "test",
						},
					},
					{
						Payload: &hcconf.HealthCheckPayload_Text{
							Text: "test2",
						},
					},
				},
			}, 5*time.Second, false, nil)

			err := hc.Check(lis.Addr().String())
			testhelpers.EventuallyAtomic(&ops).Should(Equal(1))
			Expect(err).To(BeNil())
		})
		It("should return error if not found payload", func() {
			var ops int64
			setConnHandlerListener(lis, func(conn net.Conn) {
				defer GinkgoRecover()
				_, err := conn.Write([]byte("test"))
				Expect(err).To(BeNil())
				_, err = conn.Write([]byte("toto"))
				Expect(err).To(BeNil())
				atomic.AddInt64(&ops, 1)
			})
			hc := NewTcpHealthCheck(&hcconf.TcpHealthCheck{
				Receive: []*hcconf.HealthCheckPayload{
					{
						Payload: &hcconf.HealthCheckPayload_Text{
							Text: "test",
						},
					},
					{
						Payload: &hcconf.HealthCheckPayload_Text{
							Text: "test2",
						},
					},
				},
			}, 5*time.Second, false, nil)

			err := hc.Check(lis.Addr().String())
			testhelpers.EventuallyAtomic(&ops).Should(Equal(1))
			Expect(err).ToNot(BeNil())
		})
	})
})
