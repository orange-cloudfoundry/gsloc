package gslb

import (
	"context"
	consul "github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-multierror"
	"github.com/miekg/dns"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/entries/v1"
	gslbsvc "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/services/gslb/v1"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/helpers"
	"github.com/orange-cloudfoundry/gsloc/config"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
	"strings"
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

func (s *Server) SetEntry(ctx context.Context, request *gslbsvc.SetEntryRequest) (*emptypb.Empty, error) {
	err := request.ValidateAll()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	request.Entry.Fqdn = dns.Fqdn(request.GetEntry().GetFqdn())

	signedEntry := &entries.SignedEntry{
		Entry:       request.GetEntry(),
		Healthcheck: request.GetHealthcheck(),
	}
	err = s.setSignedEntry(signedEntry)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) setSignedEntry(entry *entries.SignedEntry) error {
	sig, err := helpers.MessageSignature(entry)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to sign entry: %v", err)
	}
	entry.Signature = sig

	val, err := protojson.Marshal(entry)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to marshal entry: %v", err)
	}

	_, err = s.consulClient.KV().Put(&consul.KVPair{
		Key:   config.ConsulKVEntriesPrefix + entry.GetEntry().GetFqdn(),
		Value: val,
	}, nil)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to write entry: %v", err)
	}
	return nil
}

func (s *Server) makeTxEntry(entry *entries.SignedEntry) (*consul.TxnOp, error) {
	sig, err := helpers.MessageSignature(entry)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to sign entry: %v", err)
	}
	entry.Signature = sig

	val, err := protojson.Marshal(entry)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal entry: %v", err)
	}
	return &consul.TxnOp{
		KV: &consul.KVTxnOp{
			Verb:  consul.KVSet,
			Key:   config.ConsulKVEntriesPrefix + entry.GetEntry().GetFqdn(),
			Value: val,
		},
	}, nil
}

func (s *Server) DeleteEntry(ctx context.Context, request *gslbsvc.DeleteEntryRequest) (*emptypb.Empty, error) {
	err := request.ValidateAll()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	fqdn := dns.Fqdn(request.GetFqdn())
	_, err = s.consulClient.KV().Delete(config.ConsulKVEntriesPrefix+fqdn, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete entry: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) GetEntry(ctx context.Context, request *gslbsvc.GetEntryRequest) (*gslbsvc.GetEntryResponse, error) {
	err := request.ValidateAll()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	fqdn := dns.Fqdn(request.GetFqdn())
	pair, _, err := s.consulClient.KV().Get(config.ConsulKVEntriesPrefix+fqdn, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get entry: %v", err)
	}

	signedEntry, err := s.convertPairToSignedEntry(pair)
	if err != nil {
		return nil, err
	}
	return &gslbsvc.GetEntryResponse{
		Entry:       signedEntry.GetEntry(),
		Healthcheck: signedEntry.GetHealthcheck(),
	}, nil
}

func (s *Server) convertPairToSignedEntry(pair *consul.KVPair) (*entries.SignedEntry, error) {
	signedEntry := &entries.SignedEntry{}
	err := protojson.Unmarshal(pair.Value, signedEntry)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmarshal entry: %v", err)
	}
	return signedEntry, nil
}

func (s *Server) ListEntries(ctx context.Context, request *gslbsvc.ListEntriesRequest) (*gslbsvc.ListEntriesResponse, error) {
	err := request.ValidateAll()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	signedEnts, err := s.listEntries(request.GetPrefix(), request.GetTags())
	if err != nil {
		return nil, err
	}
	ents := make([]*gslbsvc.GetEntryResponse, len(signedEnts))
	for i, signedEnt := range signedEnts {
		ents[i] = &gslbsvc.GetEntryResponse{
			Entry:       signedEnt.GetEntry(),
			Healthcheck: signedEnt.GetHealthcheck(),
		}
	}
	return &gslbsvc.ListEntriesResponse{
		Entries: ents,
	}, nil
}

func (s *Server) listEntries(prefix string, tags []string) ([]*entries.SignedEntry, error) {
	pairs, _, err := s.consulClient.KV().List(config.ConsulKVEntriesPrefix+prefix, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list entries: %v", err)
	}

	ents := make([]*entries.SignedEntry, 0, len(pairs))
	for _, pair := range pairs {
		signedEntry, err := s.convertPairToSignedEntry(pair)
		if err != nil {
			return nil, err
		}
		hasTag := true
		for _, tag := range tags {
			if lo.Contains[string](signedEntry.GetEntry().GetTags(), tag) {
				hasTag = false
				break
			}
		}
		if !hasTag {
			continue
		}
		ents = append(ents, signedEntry)
	}
	return ents, nil
}

func (s *Server) retrieveSignedEntry(fqdn string) (*entries.SignedEntry, error) {
	pair, _, err := s.consulClient.KV().Get(config.ConsulKVEntriesPrefix+fqdn, nil)
	if err != nil {
		return nil, err
	}
	signedEntry, err := s.convertPairToSignedEntry(pair)
	if err != nil {
		return nil, err
	}
	return signedEntry, nil
}

func (s *Server) AddMember(ctx context.Context, request *gslbsvc.AddMemberRequest) (*emptypb.Empty, error) {
	err := request.ValidateAll()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	signedEntry, err := s.retrieveSignedEntry(request.GetFqdn())
	if err != nil {
		return nil, err
	}
	isIpv6 := strings.Contains(request.GetMember().GetIp(), ":")
	members := signedEntry.GetEntry().GetMembersIpv4()
	if isIpv6 {
		members = signedEntry.GetEntry().GetMembersIpv6()
	}
	for _, member := range members {
		if member.GetIp() == request.GetMember().GetIp() {
			return nil, status.Errorf(codes.AlreadyExists, "member already exists")
		}
	}
	members = append(members, request.GetMember())
	if isIpv6 {
		signedEntry.GetEntry().MembersIpv6 = members
	} else {
		signedEntry.GetEntry().MembersIpv4 = members
	}
	err = s.setSignedEntry(signedEntry)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) DeleteMember(ctx context.Context, request *gslbsvc.DeleteMemberRequest) (*emptypb.Empty, error) {
	err := request.ValidateAll()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	signedEntry, err := s.retrieveSignedEntry(request.GetFqdn())
	if err != nil {
		return nil, err
	}
	isIpv6 := strings.Contains(request.GetIp(), ":")
	members := signedEntry.GetEntry().GetMembersIpv4()
	if isIpv6 {
		members = signedEntry.GetEntry().GetMembersIpv6()
	}
	finalMembers := make([]*entries.Member, 0)
	for _, member := range members {
		if member.GetIp() != request.GetIp() {
			finalMembers = append(finalMembers, member)
		}
	}
	if isIpv6 {
		signedEntry.GetEntry().MembersIpv6 = finalMembers
	} else {
		signedEntry.GetEntry().MembersIpv4 = finalMembers
	}
	err = s.setSignedEntry(signedEntry)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) SetMemberStatus(ctx context.Context, request *gslbsvc.SetMemberStatusRequest) (*emptypb.Empty, error) {
	err := request.ValidateAll()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	signedEntry, err := s.retrieveSignedEntry(request.GetFqdn())
	if err != nil {
		return nil, err
	}
	isIpv6 := strings.Contains(request.GetIp(), ":")
	members := signedEntry.GetEntry().GetMembersIpv4()
	if isIpv6 {
		members = signedEntry.GetEntry().GetMembersIpv6()
	}
	for _, member := range members {
		if member.GetIp() == request.GetIp() {
			member.Disabled = request.Status == gslbsvc.MemberStatus_DISABLED
			break
		}
	}
	err = s.setSignedEntry(signedEntry)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) SetMembersStatusByFilter(ctx context.Context, request *gslbsvc.SetMembersStatusByFilterRequest) (*emptypb.Empty, error) {
	err := request.ValidateAll()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	signedEnts, err := s.listEntries(request.Prefix, request.Tags)
	if err != nil {
		return nil, err
	}
	var result error

	txOpts := make(consul.TxnOps, 0)

	for _, signedEnt := range signedEnts {
		updated := false
		for _, member := range signedEnt.GetEntry().GetMembersIpv4() {
			if request.GetDc() != "" && request.GetDc() != member.GetDc() {
				continue
			}
			updated = true
			member.Disabled = request.Status == gslbsvc.MemberStatus_DISABLED
		}
		for _, member := range signedEnt.GetEntry().GetMembersIpv6() {
			if request.GetDc() != "" && request.GetDc() != member.GetDc() {
				continue
			}
			updated = true
			member.Disabled = request.Status == gslbsvc.MemberStatus_DISABLED
		}
		if !updated {
			continue
		}
		txOpt, err := s.makeTxEntry(signedEnt)
		if err != nil {
			result = multierror.Append(result, err)
		}
		txOpts = append(txOpts, txOpt)
	}
	if result != nil {
		return nil, result
	}
	_, _, _, err = s.consulClient.Txn().Txn(txOpts, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update entries: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) SetHealthCheck(ctx context.Context, request *gslbsvc.SetHealthCheckRequest) (*emptypb.Empty, error) {
	err := request.ValidateAll()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	signedEntry, err := s.retrieveSignedEntry(request.GetFqdn())
	if err != nil {
		return nil, err
	}

	signedEntry.Healthcheck = request.GetHealthcheck()
	err = s.setSignedEntry(signedEntry)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) ListDcs(ctx context.Context, request *gslbsvc.ListDcsRequest) (*gslbsvc.ListDcsResponse, error) {
	err := request.ValidateAll()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

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
	return &gslbsvc.ListDcsResponse{Dcs: dcs}, nil
}
