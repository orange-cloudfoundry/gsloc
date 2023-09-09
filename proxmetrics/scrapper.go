package proxmetrics

import (
	"compress/gzip"
	"crypto/tls"
	"fmt"
	"github.com/orange-cloudfoundry/gsloc/config"
	"io"
	"net"
	"net/http"
	"time"
)

const acceptHeader = `application/openmetrics-text; version=0.0.1,text/plain;version=0.0.4;q=0.5,*/*;q=0.1`

type Scraper struct {
	httpClient *http.Client
}

func NewScraper(tlsConf *tls.Config) *Scraper {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		DualStack: true,
	}
	httpClient := &http.Client{
		Timeout: time.Second * 10,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			TLSClientConfig:       tlsConf,
		},
	}
	return &Scraper{
		httpClient: httpClient,
	}

}
func (s Scraper) Scrape(target *config.ProxyMetricsTarget) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", target.URL.URL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", acceptHeader)
	req.Header.Add("Accept-Encoding", "gzip")
	req.Header.Set("X-Prometheus-Scrape-Timeout-Seconds", fmt.Sprintf("%f", (30*time.Second).Seconds()))
	req.Header.Set("X-Proxy-Scrapping", "true")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode >= 400 && resp.StatusCode <= 499 {
			return nil, ErrNoEndpointFound(
				fmt.Sprintf(
					"Target %s (status code %d)",
					target.Name,
					resp.StatusCode,
				), target.URL.URL.String(),
			)
		}
		return nil, fmt.Errorf("server returned HTTP status %s", resp.Status)
	}

	if resp.Header.Get("Content-Encoding") != "gzip" {
		return resp.Body, nil
	}
	gzReader, err := NewReaderGzip(resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, err
	}
	return gzReader, nil
}

type ReaderGzip struct {
	main io.ReadCloser
	gzip *gzip.Reader
}

func NewReaderGzip(main io.ReadCloser) (*ReaderGzip, error) {
	gzReader, err := gzip.NewReader(main)
	if err != nil {
		return nil, err
	}
	return &ReaderGzip{
		main: main,
		gzip: gzReader,
	}, nil
}

func (r ReaderGzip) Read(p []byte) (n int, err error) {
	return r.gzip.Read(p)
}

func (r ReaderGzip) Close() error {
	r.gzip.Close()
	r.main.Close()
	return nil
}
