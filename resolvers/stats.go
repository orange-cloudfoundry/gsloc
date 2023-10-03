package resolvers

import (
	"context"
	"github.com/orange-cloudfoundry/gsloc/contexes"
	"github.com/prometheus/client_golang/prometheus"
)

var stats = metrics{
	preferred: prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gsloc",
		Subsystem: "dns_handler",
		Name:      "preferred",
		Help:      "Number of preferred response made by the dns handler",
	}, []string{
		"fqdn",
		"lb_type",
	}),

	alternate: prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gsloc",
		Subsystem: "dns_handler",
		Name:      "alternate",
		Help:      "Number of alternate response made by the dns handler",
	}, []string{
		"fqdn",
		"lb_type",
	}),

	fallback: prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gsloc",
		Subsystem: "dns_handler",
		Name:      "fallback",
		Help:      "Number of fallback response made by the dns handler",
	}, []string{
		"fqdn",
		"lb_type",
	}),

	querySuccess: prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gsloc",
		Subsystem: "dns_handler",
		Name:      "query_succeeded",
		Help:      "Number of query of type given answered",
	}, []string{
		"fqdn",
		"query_type",
		"client_ip",
	}),

	queryFailed: prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gsloc",
		Subsystem: "dns_handler",
		Name:      "query_failed",
		Help:      "Number of query of type A failing",
	}, []string{
		"fqdn",
		"query_type",
		"client_ip",
	}),
}

type metrics struct {
	preferred    *prometheus.CounterVec
	alternate    *prometheus.CounterVec
	fallback     *prometheus.CounterVec
	querySuccess *prometheus.CounterVec
	queryFailed  *prometheus.CounterVec
}

func init() {
	prometheus.MustRegister(stats.preferred)
	prometheus.MustRegister(stats.alternate)
	prometheus.MustRegister(stats.fallback)
	prometheus.MustRegister(stats.querySuccess)
	prometheus.MustRegister(stats.queryFailed)
}

func (m *metrics) AddPreferred(fqdn, lbType string) {
	m.preferred.WithLabelValues(fqdn, lbType).Add(1)
}

func (m *metrics) AddAlternate(fqdn, lbType string) {
	m.alternate.WithLabelValues(fqdn, lbType).Add(1)
}

func (m *metrics) AddFallback(fqdn, lbType string) {
	m.fallback.WithLabelValues(fqdn, lbType).Add(1)
}

func (m *metrics) AddQuerySuccess(ctx context.Context, fqdn, queryType string) {
	m.querySuccess.WithLabelValues(fqdn, queryType, contexes.GetRemoteAddr(ctx)).Add(1)
}

func (m *metrics) AddQueryFailed(ctx context.Context, fqdn, queryType string) {
	m.queryFailed.WithLabelValues(fqdn, queryType, contexes.GetRemoteAddr(ctx)).Add(1)
}
