package healthchecks

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/ArthurHlt/gohc"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/core/v1"
	hcconf "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/healthchecks/v1"
	gsloctype "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/type/v1"
	"github.com/orange-cloudfoundry/gsloc/config"
	"net/http"
)

func MakeHealthCheck(hcDef *hcconf.HealthCheck, fqdn string, plugins []*config.PluginHealthCheckConfig) (gohc.HealthChecker, error) {
	var hchecker gohc.HealthChecker
	tlsEnable := hcDef.GetTlsConfig().GetEnable()
	tlsConf := makeTlsConfig(hcDef.GetTlsConfig(), fqdn)
	switch hcDef.GetHealthChecker().(type) {
	case *hcconf.HealthCheck_GrpcHealthCheck:
		hchecker = gohc.NewGrpcHealthCheck(&gohc.GrpcOpt{
			ServiceName: hcDef.GetGrpcHealthCheck().GetServiceName(),
			Authority:   hcDef.GetGrpcHealthCheck().GetAuthority(),
			Timeout:     hcDef.GetTimeout().AsDuration(),
			TlsEnabled:  tlsEnable,
			TlsConfig:   tlsConf,
		})
	case *hcconf.HealthCheck_HttpHealthCheck:
		hchecker = gohc.NewHttpHealthCheck(&gohc.HttpOpt{
			Host:             hcDef.GetHttpHealthCheck().GetHost(),
			Path:             hcDef.GetHttpHealthCheck().GetPath(),
			Send:             makeGohcPayload(hcDef.GetHttpHealthCheck().GetSend()),
			Receive:          makeGohcPayload(hcDef.GetHttpHealthCheck().GetReceive()),
			Headers:          makeGohcHeader(hcDef.GetHttpHealthCheck().GetRequestHeadersToAdd()),
			ExpectedStatuses: makeGohcExpectedStatus(hcDef.GetHttpHealthCheck().GetExpectedStatuses()),
			CodecClientType:  gohc.CodecClientType(hcDef.GetHttpHealthCheck().GetCodecClientType()),
			Method:           hcconf.RequestMethod_name[int32(hcDef.GetHttpHealthCheck().GetMethod())],
			Timeout:          hcDef.GetTimeout().AsDuration(),
			TlsEnabled:       tlsEnable,
			TlsConfig:        tlsConf,
		})
	case *hcconf.HealthCheck_TcpHealthCheck:
		hchecker = gohc.NewTcpHealthCheck(&gohc.TcpOpt{
			Send:       makeGohcPayload(hcDef.GetTcpHealthCheck().GetSend()),
			Receive:    makeGohcPayloads(hcDef.GetTcpHealthCheck().GetReceive()),
			Timeout:    hcDef.GetTimeout().AsDuration(),
			TlsEnabled: tlsEnable,
			TlsConfig:  tlsConf,
		})
	case *hcconf.HealthCheck_IcmpHealthCheck:
		hchecker = gohc.NewIcmpHealthCheck(&gohc.IcmpOpt{
			Timeout: hcDef.GetTimeout().AsDuration(),
			Delay:   hcDef.GetIcmpHealthCheck().GetDelay().AsDuration(),
		})
	case *hcconf.HealthCheck_UdpHealthCheck:
		hchecker = gohc.NewUdpHealthCheck(&gohc.UdpOpt{
			Send:        makeGohcPayload(hcDef.GetUdpHealthCheck().GetSend()),
			Receive:     makeGohcPayloads(hcDef.GetUdpHealthCheck().GetReceive()),
			PingTimeout: hcDef.GetUdpHealthCheck().GetPingTimeout().AsDuration(),
			Timeout:     hcDef.GetTimeout().AsDuration(),
			Delay:       hcDef.GetUdpHealthCheck().GetDelay().AsDuration(),
		})
	case *hcconf.HealthCheck_NoHealthCheck:
		hchecker = gohc.NewNoHealthCheck()
	case *hcconf.HealthCheck_PluginHealthCheck:
		var foundPlugin *config.PluginHealthCheckConfig
		for _, plugin := range plugins {
			if hcDef.GetPluginHealthCheck().Name == plugin.Name {
				foundPlugin = plugin
				break
			}
		}
		if foundPlugin == nil {
			return nil, fmt.Errorf("plugin %s not found", hcDef.GetPluginHealthCheck().GetName())
		}
		cas := make([]string, 0)
		if hcDef.GetTlsConfig().GetCa() != "" {
			cas = append(cas, hcDef.GetTlsConfig().GetCa())
		}
		hchecker = gohc.NewProgramHealthCheck(&gohc.ProgramOpt{
			Path:       foundPlugin.Path,
			Args:       foundPlugin.Args,
			Options:    hcDef.GetPluginHealthCheck().GetOptions().AsMap(),
			Timeout:    hcDef.GetTimeout().AsDuration(),
			TlsEnabled: tlsEnable,
			ProgramTlsConfig: &gohc.ProgramTlsOpt{
				InsecureSkipVerify: false,
				ServerName:         hcDef.GetTlsConfig().GetServerName(),
				RootCAs:            cas,
			},
		})
	}
	return hchecker, nil
}

func makeGohcPayload(hcPayload *hcconf.HealthCheckPayload) *gohc.Payload {
	if hcPayload == nil {
		return nil
	}
	return &gohc.Payload{
		Text:   hcPayload.GetText(),
		Binary: hcPayload.GetBinary(),
	}
}

func makeGohcHeader(hcHeaders []*core.HeaderValueOption) http.Header {
	headers := make(http.Header)
	for _, hcHeader := range hcHeaders {
		if !hcHeader.GetAppend() {
			headers.Set(hcHeader.GetHeader().GetKey(), hcHeader.GetHeader().GetValue())
			continue
		}
		headers.Add(hcHeader.GetHeader().GetKey(), hcHeader.GetHeader().GetValue())
	}
	return headers
}

func makeGohcPayloads(hcPayloads []*hcconf.HealthCheckPayload) []*gohc.Payload {
	if len(hcPayloads) == 0 {
		return nil
	}
	payloads := make([]*gohc.Payload, len(hcPayloads))
	for i, hcPayload := range hcPayloads {
		payloads[i] = makeGohcPayload(hcPayload)
	}
	return payloads
}

func makeGohcExpectedStatus(rn *gsloctype.Int64Range) *gohc.IntRange {
	if rn == nil {
		return nil
	}
	return &gohc.IntRange{
		Start: rn.GetStart(),
		End:   rn.GetEnd(),
	}
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
