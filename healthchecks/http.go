package healthchecks

import (
	"bytes"
	"crypto/tls"
	"fmt"
	hcconf "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/healthchecks/v1"
	gsloctype "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/type/v1"
	"github.com/quic-go/quic-go/http3"
	"io"
	"net"
	"net/http"
	"time"
)

type HttpHealthCheck struct {
	httpClient *http.Client
	httpHcConf *hcconf.HttpHealthCheck
	tlsEnabled bool
}

func NewHttpHealthCheck(httpHcConf *hcconf.HttpHealthCheck, timeout time.Duration, tlsEnabled bool, tlsConf *tls.Config) *HttpHealthCheck {
	return &HttpHealthCheck{
		httpClient: makeHttpClient(httpHcConf.CodecClientType, tlsConf, timeout),
		httpHcConf: httpHcConf,
		tlsEnabled: tlsEnabled,
	}
}

func (h *HttpHealthCheck) Check(host string) error {
	protocol := "http"
	if h.tlsEnabled {
		protocol = "https"
	}
	url := fmt.Sprintf("%s://%s%s", protocol, host, h.httpHcConf.Path)

	method := http.MethodGet
	if h.httpHcConf.Method != hcconf.RequestMethod_METHOD_UNSPECIFIED {
		method = hcconf.RequestMethod_name[int32(h.httpHcConf.Method)]
	}
	var body io.Reader
	if h.httpHcConf.Send != nil {
		body = bytes.NewReader(h.httpHcConf.Send.GetData())
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}
	if h.httpHcConf.Host != "" {
		req.Host = h.httpHcConf.Host
	}
	req.Header.Set("User-Agent", "lbaas-minion")
	for _, header := range h.httpHcConf.GetRequestHeadersToAdd() {
		if header.GetAppend() {
			req.Header.Add(header.GetHeader().GetKey(), header.GetHeader().GetValue())
		} else {
			req.Header.Set(header.GetHeader().GetKey(), header.GetHeader().GetValue())
		}
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	start := int64(200)
	end := int64(201)
	if h.httpHcConf.GetExpectedStatuses() != nil {
		start = h.httpHcConf.GetExpectedStatuses().GetStart()
		end = h.httpHcConf.GetExpectedStatuses().GetEnd()
	}
	statusCode := int64(resp.StatusCode)
	if statusCode < start || statusCode >= end {
		return fmt.Errorf("unexpected status code, got %d not in range [%d, %d)", statusCode, start, end)
	}
	if h.httpHcConf.Receive != nil {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %v", err)
		}
		if !bytes.Contains(b, h.httpHcConf.Receive.GetData()) {
			return fmt.Errorf("response body does not contains expected data")
		}
	}
	return nil
}

func makeHttpClient(codec gsloctype.CodecClientType, tlsConf *tls.Config, timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	var roundTripper http.RoundTripper
	switch codec {
	case gsloctype.CodecClientType_HTTP3:
		roundTripper = &http3.RoundTripper{
			TLSClientConfig: tlsConf,
		}
	default:
		roundTripper = &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			TLSClientConfig:       tlsConf,
		}
	}
	httpClient := &http.Client{
		Transport: roundTripper,
		Timeout:   timeout,
	}
	return httpClient
}
