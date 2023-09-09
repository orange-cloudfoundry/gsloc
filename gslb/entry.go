package gslb

import (
	"context"
	consul "github.com/hashicorp/consul/api"
	"github.com/miekg/dns"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/entries/v1"
	gslbsvc "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/services/gslb/v1"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/helpers"
	"github.com/orange-cloudfoundry/gsloc/config"
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

	request.Entry.Fqdn = dns.CanonicalName(request.GetEntry().GetFqdn())

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

func (s *Server) ListEntriesStatus(ctx context.Context, req *gslbsvc.ListEntriesStatusRequest) (*gslbsvc.ListEntriesStatusResponse, error) {
	allEntriesStatus, err := s.gslocConsul.ListEntriesStatus(req.GetPrefix(), req.GetTags())
	if err != nil {
		return nil, err
	}
	return &gslbsvc.ListEntriesStatusResponse{
		EntriesStatus: allEntriesStatus,
	}, nil
}

func (s *Server) GetEntryStatus(ctx context.Context, req *gslbsvc.GetEntryStatusRequest) (*gslbsvc.GetEntryStatusResponse, error) {
	fqdn := dns.CanonicalName(req.GetFqdn())
	return s.gslocConsul.GetEntryStatus(fqdn)
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

	fqdn := dns.CanonicalName(request.GetFqdn())
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

	fqdn := dns.CanonicalName(request.GetFqdn())
	pair, _, err := s.consulClient.KV().Get(config.ConsulKVEntriesPrefix+fqdn, nil)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "entry not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get entry: %v", err)
	}

	signedEntry, err := s.gslocConsul.ConvertPairToSignedEntry(pair)
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

	fqdn := dns.CanonicalName(request.GetFqdn())
	pair, _, err := s.consulClient.KV().Get(config.ConsulKVEntriesPrefix+fqdn, nil)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "entry not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get entry: %v", err)
	}

	signedEntry, err := s.gslocConsul.ConvertPairToSignedEntry(pair)
	if err != nil {
		return nil, err
	}
	return &gslbsvc.GetEntryResponse{
		Entry:       signedEntry.GetEntry(),
		Healthcheck: signedEntry.GetHealthcheck(),
	}, nil
}

func (s *Server) ListEntries(ctx context.Context, request *gslbsvc.ListEntriesRequest) (*gslbsvc.ListEntriesResponse, error) {
	err := request.ValidateAll()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	signedEnts, err := s.gslocConsul.ListEntries(request.GetPrefix(), request.GetTags())
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
