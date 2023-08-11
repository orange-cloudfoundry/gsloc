package gslb

import (
	consul "github.com/hashicorp/consul/api"
	gslbsvc "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/services/gslb/v1"
)

type Server struct {
	consulClient *consul.Client
	gslbsvc.UnimplementedGSLBServer
}

func NewServer(consulClient *consul.Client) (*Server, error) {
	s := &Server{
		consulClient: consulClient,
	}
	return s, nil
}
