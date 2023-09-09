package proxmetrics

import (
	gslbsvc "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/services/gslb/v1"
	"github.com/orange-cloudfoundry/gsloc/disco"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"net/http"
)

type StatusCollector struct {
	desc        *prometheus.Desc
	gslocConsul *disco.GslocConsul
}

func StatusHandler(collector *StatusCollector) http.Handler {
	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)
	return promhttp.InstrumentMetricHandler(
		registry, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
	)
}

func NewStatusCollector(gslocConsul *disco.GslocConsul) *StatusCollector {
	return &StatusCollector{
		gslocConsul: gslocConsul,
		desc: prometheus.NewDesc(
			"gsloc_entry_status",
			"Get entry status information. 0 means online, 1 means check failed, 2 means offline (disabled by user), 3 means unknown.",
			[]string{"fqdn", "member_ip", "dc", "type"},
			nil,
		),
	}
}

func (s *StatusCollector) Describe(descs chan<- *prometheus.Desc) {
	descs <- s.desc
}

func (s *StatusCollector) Collect(metrics chan<- prometheus.Metric) {
	entriesStatus, err := s.gslocConsul.ListEntriesStatus("", []string{})
	if err != nil {
		log.WithError(err).Error("Failed to list entries status")
		return
	}
	for _, entryStatus := range entriesStatus {
		for _, entry := range entryStatus.MembersIpv4 {
			metrics <- s.memberStatusToProm(entryStatus.Fqdn, entry, true)
		}
		for _, entry := range entryStatus.MembersIpv6 {
			metrics <- s.memberStatusToProm(entryStatus.Fqdn, entry, false)
		}
	}
}

func (s *StatusCollector) memberStatusToProm(fqdn string, ms *gslbsvc.MemberStatus, isIpv4 bool) prometheus.Metric {
	val := 3
	switch ms.GetStatus() {
	case gslbsvc.MemberStatus_ONLINE:
		val = 0
	case gslbsvc.MemberStatus_OFFLINE:
		val = 2
	case gslbsvc.MemberStatus_CHECK_FAILED:
		val = 1
	}
	typeMember := "ipv6"
	if isIpv4 {
		typeMember = "ipv4"
	}
	return prometheus.MustNewConstMetric(
		s.desc,
		prometheus.GaugeValue,
		float64(val),
		fqdn,
		ms.GetIp(),
		ms.GetDc(),
		typeMember,
	)
}
