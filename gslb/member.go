package gslb

import (
	"context"
	consul "github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-multierror"
	"github.com/miekg/dns"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/entries/v1"
	gslbsvc "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/services/gslb/v1"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"strings"
)

const maxTransactions = 64

func (s *Server) SetMember(ctx context.Context, request *gslbsvc.SetMemberRequest) (*emptypb.Empty, error) {
	err := request.ValidateAll()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	dcs, err := s.listDcs()
	if err != nil {
		return nil, err
	}

	if !lo.Contains[string](dcs, request.GetMember().GetDc()) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid dc: %s", request.GetMember().GetDc())
	}

	fqdn := dns.Fqdn(request.GetFqdn())

	signedEntry, err := s.retrieveSignedEntry(fqdn)
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

	fqdn := dns.Fqdn(request.GetFqdn())

	signedEntry, err := s.retrieveSignedEntry(fqdn)
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

func (s *Server) setStatusMember(fqdn string, members []*entries.Member, request *gslbsvc.SetMembersStatusRequest, mapToUpdate map[string][]string) bool {
	updated := false
	for _, member := range members {
		if request.GetDc() != "" && request.GetDc() != member.GetDc() {
			continue
		}
		if request.GetIp() != "" && request.GetIp() != member.GetIp() {
			continue
		}
		if _, ok := mapToUpdate[fqdn]; !ok {
			mapToUpdate[fqdn] = make([]string, 0)
		}
		mapToUpdate[fqdn] = append(mapToUpdate[fqdn], member.GetIp())
		updated = true
		member.Disabled = request.Status == gslbsvc.MemberState_DISABLED
	}
	return updated
}

func (s *Server) SetMembersStatus(ctx context.Context, request *gslbsvc.SetMembersStatusRequest) (*gslbsvc.SetMembersStatusResponse, error) {
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
	mapToUpdate := make(map[string][]string)
	for _, signedEnt := range signedEnts {
		fqdn := signedEnt.GetEntry().GetFqdn()
		updatedIpv4 := s.setStatusMember(fqdn, signedEnt.GetEntry().GetMembersIpv4(), request, mapToUpdate)
		updatedIpv6 := s.setStatusMember(fqdn, signedEnt.GetEntry().GetMembersIpv6(), request, mapToUpdate)
		if !updatedIpv4 && !updatedIpv6 {
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
	if len(txOpts) == 0 {
		return &gslbsvc.SetMembersStatusResponse{}, nil
	}
	if !request.DryRun {
		err = s.applyTx(txOpts)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to update entries: %v", err)
		}
	}

	infos := make([]*gslbsvc.SetMembersStatusResponse_Info, 0)
	for fqdn, ips := range mapToUpdate {
		infos = append(infos, &gslbsvc.SetMembersStatusResponse_Info{
			Fqdn: fqdn,
			Ips:  ips,
		})
	}

	return &gslbsvc.SetMembersStatusResponse{
		Updated: infos,
	}, nil
}

func (s *Server) applyTx(opts consul.TxnOps) error {
	var result error
	if len(opts) == 0 {
		return nil
	}
	todo := int(len(opts) / maxTransactions)
	for i := 0; i <= todo; i++ {
		max := (i + 1) * 64
		if max > len(opts) {
			max = len(opts)
		}
		subTxOpts := opts[i*64 : max]
		_, _, _, err := s.consulClient.Txn().Txn(subTxOpts, nil)
		if err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result
}

func (s *Server) GetMember(ctx context.Context, request *gslbsvc.GetMemberRequest) (*gslbsvc.GetMemberResponse, error) {
	err := request.ValidateAll()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	fqdn := dns.Fqdn(request.GetFqdn())

	signedEntry, err := s.retrieveSignedEntry(fqdn)
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
			return &gslbsvc.GetMemberResponse{
				Member: member,
			}, nil
		}
	}
	return nil, status.Errorf(codes.NotFound, "member not found")
}

func (s *Server) ListMembers(ctx context.Context, request *gslbsvc.ListMembersRequest) (*gslbsvc.ListMembersResponse, error) {
	err := request.ValidateAll()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	fqdn := dns.Fqdn(request.GetFqdn())

	signedEntry, err := s.retrieveSignedEntry(fqdn)
	if err != nil {
		return nil, err
	}

	return &gslbsvc.ListMembersResponse{
		MembersIpv4: signedEntry.GetEntry().GetMembersIpv4(),
		MembersIpv6: signedEntry.GetEntry().GetMembersIpv6(),
	}, nil
}
