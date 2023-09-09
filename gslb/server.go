package gslb

import (
	consul "github.com/hashicorp/consul/api"
	gslbsvc "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/services/gslb/v1"
	"github.com/orange-cloudfoundry/gsloc/disco"
)

type Server struct {
	consulClient *consul.Client
	gslocConsul  *disco.GslocConsul
	gslbsvc.UnimplementedGSLBServer
}

func NewServer(consulClient *consul.Client, gslocConsul *disco.GslocConsul) (*Server, error) {
	s := &Server{
		consulClient: consulClient,
		gslocConsul:  gslocConsul,
	}
	return s, nil
}
