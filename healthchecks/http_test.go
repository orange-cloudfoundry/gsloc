package healthchecks_test

import (
	"crypto/tls"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/core/v1"
	hcconf "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/healthchecks/v1"
	gsloctype "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/type/v1"
	. "github.com/orange-cloudfoundry/gsloc/healthchecks"
	"io"
	"net/http"
	"time"
)

var _ = Describe("Http", func() {
	Context("Check", func() {
		Context("Basic", func() {
			var server *ghttp.Server
			BeforeEach(func() {
				server = ghttp.NewServer()
			})
			AfterEach(func() {
				//shut down the httpServer between tests
				server.Close()
			})
			It("should return nil on the most basic test on path / and 200 status code", func() {
				server.AppendHandlers(ghttp.RespondWith(200, "OK"))

				hc := NewHttpHealthCheck(&hcconf.HttpHealthCheck{
					Path: "/",
				}, 5*time.Second, false, nil)

				err := hc.Check(urlToHost(server.URL()))
				Expect(err).To(BeNil())
			})
			It("should return an error when status code is not expected", func() {
				server.AppendHandlers(ghttp.RespondWith(404, "NOT FOUND"))

				hc := NewHttpHealthCheck(&hcconf.HttpHealthCheck{
					Path: "/",
				}, 5*time.Second, false, nil)

				err := hc.Check(urlToHost(server.URL()))
				Expect(err).ToNot(BeNil())
				Expect(err.Error()).To(ContainSubstring("404"))
			})
			It("should return an error when take more time than timeout", func() {
				server.AppendHandlers(func(w http.ResponseWriter, req *http.Request) {
					time.Sleep(10 * time.Millisecond)
				})

				hc := NewHttpHealthCheck(&hcconf.HttpHealthCheck{
					Path: "/",
				}, 1*time.Nanosecond, false, nil)

				err := hc.Check(urlToHost(server.URL()))
				Expect(err).ToNot(BeNil())
				Expect(err.Error()).To(ContainSubstring("context deadline exceeded"))
			})
			It("should return nil when status code is in expected range", func() {
				server.AppendHandlers(ghttp.RespondWith(404, "NOT FOUND"))

				hc := NewHttpHealthCheck(&hcconf.HttpHealthCheck{
					Path: "/",
					ExpectedStatuses: &gsloctype.Int64Range{
						Start: 400,
						End:   500,
					},
				}, 5*time.Second, false, nil)

				err := hc.Check(urlToHost(server.URL()))
				Expect(err).To(BeNil())
			})
			It("should append headers when user declare it", func() {
				server.AppendHandlers(func(w http.ResponseWriter, req *http.Request) {
					Expect(req.Header.Get("X-Test")).To(Equal("test"))
					Expect(req.Header.Values("X-Test-Append")).To(Equal([]string{"test1", "test2"}))
				})

				hc := NewHttpHealthCheck(&hcconf.HttpHealthCheck{
					Path: "/",
					RequestHeadersToAdd: []*core.HeaderValueOption{
						{
							Header: &core.HeaderValue{
								Key:   "X-Test",
								Value: "test",
							},
							Append: false,
						},
						{
							Header: &core.HeaderValue{
								Key:   "X-Test-Append",
								Value: "test1",
							},
							Append: false,
						},
						{
							Header: &core.HeaderValue{
								Key:   "X-Test-Append",
								Value: "test2",
							},
							Append: true,
						},
					},
				}, 5*time.Second, false, nil)

				err := hc.Check(urlToHost(server.URL()))
				Expect(err).To(BeNil())
				Expect(server.ReceivedRequests()).Should(HaveLen(1))
			})
			It("should set different method when user declare it", func() {
				server.AppendHandlers(func(w http.ResponseWriter, req *http.Request) {
					Expect(req.Method).To(Equal("POST"))
				})

				hc := NewHttpHealthCheck(&hcconf.HttpHealthCheck{
					Path:   "/",
					Method: hcconf.RequestMethod_POST,
				}, 5*time.Second, false, nil)

				err := hc.Check(urlToHost(server.URL()))
				Expect(err).To(BeNil())
				Expect(server.ReceivedRequests()).Should(HaveLen(1))
			})
			It("should set different host when user declare it", func() {
				server.AppendHandlers(func(w http.ResponseWriter, req *http.Request) {
					Expect(req.Host).To(Equal("myhost"))
				})

				hc := NewHttpHealthCheck(&hcconf.HttpHealthCheck{
					Host: "myhost",
					Path: "/",
				}, 5*time.Second, false, nil)

				err := hc.Check(urlToHost(server.URL()))
				Expect(err).To(BeNil())
				Expect(server.ReceivedRequests()).Should(HaveLen(1))
			})
			When("User set send/receive payload", func() {
				It("should return nil if received payload contains what user wanted", func() {
					server.AppendHandlers(ghttp.RespondWith(200, "long text contains ok here"))

					hc := NewHttpHealthCheck(&hcconf.HttpHealthCheck{
						Path: "/",
						Receive: &hcconf.HealthCheckPayload{
							Payload: &hcconf.HealthCheckPayload_Text{
								Text: "ok",
							},
						},
					}, 5*time.Second, false, nil)

					err := hc.Check(urlToHost(server.URL()))
					Expect(err).To(BeNil())
				})
				It("should return an error if received payload doesn't contains what user wanted", func() {
					server.AppendHandlers(ghttp.RespondWith(200, "well not here"))

					hc := NewHttpHealthCheck(&hcconf.HttpHealthCheck{
						Path: "/",
						Receive: &hcconf.HealthCheckPayload{
							Payload: &hcconf.HealthCheckPayload_Text{
								Text: "ok",
							},
						},
					}, 5*time.Second, false, nil)

					err := hc.Check(urlToHost(server.URL()))
					Expect(err).ToNot(BeNil())
					Expect(err.Error()).To(ContainSubstring("not contains"))
				})
				It("should send payload given by user", func() {
					server.AppendHandlers(func(w http.ResponseWriter, req *http.Request) {
						body, err := io.ReadAll(req.Body)
						Expect(err).To(BeNil())
						Expect(string(body)).To(Equal("ok"))
					})

					hc := NewHttpHealthCheck(&hcconf.HttpHealthCheck{
						Path: "/",
						Send: &hcconf.HealthCheckPayload{
							Payload: &hcconf.HealthCheckPayload_Text{
								Text: "ok",
							},
						},
					}, 5*time.Second, false, nil)

					err := hc.Check(urlToHost(server.URL()))
					Expect(err).To(BeNil())
					Expect(server.ReceivedRequests()).Should(HaveLen(1))
				})
			})
		})
	})

	Context("Https Server", func() {
		var server *ghttp.Server
		BeforeEach(func() {
			server = ghttp.NewTLSServer()
		})
		AfterEach(func() {
			//shut down the httpServer between tests
			server.Close()
		})
		It("should return an error if tls not enabled", func() {
			server.AppendHandlers(ghttp.RespondWith(200, "OK"))

			hc := NewHttpHealthCheck(&hcconf.HttpHealthCheck{
				Path: "/",
			}, 5*time.Second, false, nil)

			err := hc.Check(urlToHost(server.URL()))
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("400"))
		})
		It("should return nil on the most basic test on path / and 200 status code", func() {
			server.AppendHandlers(ghttp.RespondWith(200, "OK"))

			hc := NewHttpHealthCheck(&hcconf.HttpHealthCheck{
				Path: "/",
			}, 5*time.Second, true,
				&tls.Config{
					InsecureSkipVerify: true,
				})

			err := hc.Check(urlToHost(server.URL()))
			Expect(err).To(BeNil())
		})
	})

})
