package gslb

import (
	consul "github.com/hashicorp/consul/api"
	gslbsvc "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/services/gslb/v1"
	"github.com/orange-cloudfoundry/gsloc/config"
	"github.com/orange-cloudfoundry/gsloc/disco"
)

type Server struct {
	consulClient *consul.Client
	gslocConsul  *disco.GslocConsul
	hcPlugins    []*config.PluginHealthCheckConfig
	gslbsvc.UnimplementedGSLBServer
}

func NewServer(consulClient *consul.Client, gslocConsul *disco.GslocConsul, plugins []*config.PluginHealthCheckConfig) (*Server, error) {
	s := &Server{
		consulClient: consulClient,
		gslocConsul:  gslocConsul,
		hcPlugins:    plugins,
	}
	return s, nil
}
