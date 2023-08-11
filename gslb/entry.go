package gslb

import (
	"context"
	consul "github.com/hashicorp/consul/api"
	"github.com/miekg/dns"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/entries/v1"
	gslbsvc "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/services/gslb/v1"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/helpers"
	"github.com/orange-cloudfoundry/gsloc/config"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
	"strings"
)

func (s *Server) SetEntry(ctx context.Context, request *gslbsvc.SetEntryRequest) (*emptypb.Empty, error) {
	err := request.ValidateAll()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	dcs, err := s.listDcs()
	if err != nil {
		return nil, err
	}
	err = s.checkMembersInDCS(request.GetEntry(), dcs)
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

func (s *Server) GetEntryStatus(ctx context.Context, req *gslbsvc.GetEntryStatusRequest) (*gslbsvc.GetEntryStatusResponse, error) {
	fqdn := dns.Fqdn(req.GetFqdn())
	pair, _, err := s.consulClient.KV().Get(config.ConsulKVEntriesPrefix+fqdn, nil)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "entry not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get entry: %v", err)
	}

	signedEntry, err := s.convertPairToSignedEntry(pair)
	if err != nil {
		return nil, err
	}

	resp := &gslbsvc.GetEntryStatusResponse{
		Fqdn:        fqdn,
		MembersIpv4: make([]*gslbsvc.MemberStatus, 0),
		MembersIpv6: make([]*gslbsvc.MemberStatus, 0),
	}
	msMap := map[string]*gslbsvc.MemberStatus{}
	for _, member := range signedEntry.GetEntry().GetMembersIpv4() {
		ms := &gslbsvc.MemberStatus{
			Ip:            member.GetIp(),
			Dc:            member.GetDc(),
			Status:        gslbsvc.MemberStatus_OFFLINE,
			FailureReason: "",
		}
		msMap[fqdn+ms.GetIp()] = ms
		resp.MembersIpv4 = append(resp.MembersIpv4, ms)
	}
	for _, member := range signedEntry.GetEntry().GetMembersIpv6() {
		ms := &gslbsvc.MemberStatus{
			Ip:            member.GetIp(),
			Dc:            member.GetDc(),
			Status:        gslbsvc.MemberStatus_OFFLINE,
			FailureReason: "",
		}
		msMap[fqdn+ms.GetIp()] = ms
		resp.MembersIpv4 = append(resp.MembersIpv4, ms)
	}

	ents, _, err := s.consulClient.Health().Service(fqdn, "", false, &consul.QueryOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "entry not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get health: %v", err)
	}
	for _, ent := range ents {
		ms, ok := msMap[fqdn+ent.Service.Address]
		if !ok {
			continue
		}
		var check *consul.HealthCheck
		for _, c := range ent.Checks {
			if c.Type == "http" {
				check = c
				break
			}
		}
		if check == nil {
			log.Warnf("no http check found for %s", ent.Service.Address)
			continue
		}
		if check.Status == consul.HealthPassing {
			ms.Status = gslbsvc.MemberStatus_ONLINE
		} else {
			ms.Status = gslbsvc.MemberStatus_CHECK_FAILED
			ms.FailureReason = check.Output
		}
	}
	return resp, nil
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
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "entry not found")
		}
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

func (s *Server) GetEntryWithStatus(ctx context.Context, request *gslbsvc.GetEntryRequest) (*gslbsvc.GetEntryResponse, error) {
	err := request.ValidateAll()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	fqdn := dns.Fqdn(request.GetFqdn())
	pair, _, err := s.consulClient.KV().Get(config.ConsulKVEntriesPrefix+fqdn, nil)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "entry not found")
		}
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
	if pair == nil {
		return nil, status.Errorf(codes.NotFound, "entry not found")
	}
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
			if !lo.Contains[string](signedEntry.GetEntry().GetTags(), tag) {
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
