package gslb

import (
	"context"
	consul "github.com/hashicorp/consul/api"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/entries/v1"
	gslbsvc "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/services/gslb/v1"
	"github.com/orange-cloudfoundry/gsloc/config"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sort"
)

func (s *Server) listDcs() ([]string, error) {
	nodes, _, err := s.consulClient.Catalog().Nodes(&consul.QueryOptions{})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list dcs: %v", err)
	}
	dcsMap := make(map[string]struct{}, 0)
	for _, node := range nodes {
		dc, ok := node.Meta[config.ConsulMetaDcKey]
		if !ok {
			continue
		}
		dcsMap[dc] = struct{}{}
	}
	dcs := make([]string, 0)
	for dc := range dcsMap {
		dcs = append(dcs, dc)
	}
	return dcs, nil
}

func (s *Server) checkMembersInDCS(entry *entries.Entry, dcs []string) error {
	for _, member := range entry.GetMembersIpv4() {
		if !lo.Contains[string](dcs, member.Dc) {
			return status.Errorf(codes.NotFound, "no member in dc %s", member.Dc)
		}
	}
	for _, member := range entry.GetMembersIpv6() {
		if !lo.Contains[string](dcs, member.Dc) {
			return status.Errorf(codes.NotFound, "no member in dc %s", member.Dc)
		}
	}
	return nil
}

func (s *Server) ListDcs(ctx context.Context, request *gslbsvc.ListDcsRequest) (*gslbsvc.ListDcsResponse, error) {
	err := request.ValidateAll()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	dcs, err := s.listDcs()
	if err != nil {
		return nil, err
	}
	sort.Strings(dcs)
	return &gslbsvc.ListDcsResponse{Dcs: dcs}, nil
}
