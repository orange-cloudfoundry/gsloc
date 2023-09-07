package healthchecks

import (
	"crypto/tls"
	"crypto/x509"
	hcconf "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/healthchecks/v1"
)

//go:generate mockgen -destination=../testhelpers/mock_healthchecker.go -package=testhelpers . HealthChecker

type HealthChecker interface {
	Check(host string) error
}

func MakeHealthCheck(hcDef *hcconf.HealthCheck, fqdn string) HealthChecker {
	var hchecker HealthChecker
	tlsEnable := hcDef.GetTlsConfig().GetEnable()
	tlsConf := makeTlsConfig(hcDef.GetTlsConfig(), fqdn)
	switch hcDef.GetHealthChecker().(type) {
	case *hcconf.HealthCheck_GrpcHealthCheck:
		hchecker = NewGrpcHealthCheck(
			hcDef.GetGrpcHealthCheck(),
			hcDef.GetTimeout().AsDuration(),
			tlsEnable,
			tlsConf,
		)
	case *hcconf.HealthCheck_HttpHealthCheck:
		hchecker = NewHttpHealthCheck(
			hcDef.GetHttpHealthCheck(),
			hcDef.GetTimeout().AsDuration(),
			tlsEnable,
			tlsConf,
		)
	case *hcconf.HealthCheck_TcpHealthCheck:
		hchecker = NewTcpHealthCheck(
			hcDef.GetTcpHealthCheck(),
			hcDef.GetTimeout().AsDuration(),
			tlsEnable,
			tlsConf,
		)
	case *hcconf.HealthCheck_NoHealthCheck:
		hchecker = NewNoHealthCheck()
	}
	return hchecker
}

func makeTlsConfig(tlsConf *hcconf.TlsConfig, fqdn string) *tls.Config {
	if tlsConf == nil || !tlsConf.Enable {
		return nil
	}
	if fqdn[len(fqdn)-1] == '.' {
		fqdn = fqdn[:len(fqdn)-1]
	}
	serverName := fqdn
	if tlsConf.GetServerName() != "" {
		serverName = tlsConf.GetServerName()
	}
	var caCertPool *x509.CertPool
	if tlsConf.GetCa() != "" {
		caCertPool = x509.NewCertPool()
		caCertPool.AppendCertsFromPEM([]byte(tlsConf.GetCa()))
	}
	return &tls.Config{
		RootCAs:    caCertPool,
		ServerName: serverName,
	}
}
