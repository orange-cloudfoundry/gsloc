package healthchecks

import (
	"crypto/tls"
	hcconf "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/healthchecks/v1"
)

//go:generate mockgen -destination=../testhelpers/mock_healthchecker.go -package=testhelpers . HealthChecker

type HealthChecker interface {
	Check(host string) error
}

func MakeHealthCheck(hcDef *hcconf.HealthCheck, tlsConf *tls.Config) HealthChecker {
	var hchecker HealthChecker
	switch hcDef.GetHealthChecker().(type) {
	case *hcconf.HealthCheck_GrpcHealthCheck:
		hchecker = NewGrpcHealthCheck(
			hcDef.GetGrpcHealthCheck(),
			hcDef.GetTimeout().AsDuration(),
			hcDef.GetEnableTls(),
			tlsConf,
		)
	case *hcconf.HealthCheck_HttpHealthCheck:
		hchecker = NewHttpHealthCheck(
			hcDef.GetHttpHealthCheck(),
			hcDef.GetTimeout().AsDuration(),
			hcDef.GetEnableTls(),
			tlsConf,
		)
	case *hcconf.HealthCheck_TcpHealthCheck:
		hchecker = NewTcpHealthCheck(
			hcDef.GetTcpHealthCheck(),
			hcDef.GetTimeout().AsDuration(),
			hcDef.GetEnableTls(),
			tlsConf,
		)
	case *hcconf.HealthCheck_NoHealthCheck:
		hchecker = NewNoHealthCheck()
	}
	return hchecker
}
