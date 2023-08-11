package resolvers

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/hashicorp/go-multierror"
	"github.com/miekg/dns"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/entries/v1"
	"github.com/orange-cloudfoundry/gsloc/config"
	"github.com/orange-cloudfoundry/gsloc/contexes"
	"github.com/orange-cloudfoundry/gsloc/lb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/protojson"
	"net"
	"sync"
)

const (
	defaultTtl    = 60
	allMemberHost = "_all."
)

type entryRef struct {
	entry       *entries.Entry
	lbPreferred lb.Loadbalancer
	lbAlternate lb.Loadbalancer
	lbFallback  lb.Loadbalancer
}

type GSLBHandler struct {
	entries        *sync.Map
	lbFactory      *lb.LBFactory
	allowedInspect []*config.CIDR
}

func NewGSLBHandler(lbFactory *lb.LBFactory, allowedInspect []*config.CIDR) *GSLBHandler {
	return &GSLBHandler{
		entries:        &sync.Map{},
		lbFactory:      lbFactory,
		allowedInspect: allowedInspect,
	}
}

func (h *GSLBHandler) SetCatalogEntry(entry *entries.Entry) {
	h.entries.Store(entry.Fqdn, entryRef{
		entry:       entry,
		lbPreferred: h.lbFactory.MakeLb(entry, entry.GetLbAlgoPreferred()),
		lbAlternate: h.lbFactory.MakeLb(entry, entry.GetLbAlgoAlternate()),
		lbFallback:  h.lbFactory.MakeLb(entry, entry.GetLbAlgoFallback()),
	})
}

func (h *GSLBHandler) RemoveCatalogEntry(entry *entries.Entry) {
	h.entries.Delete(entry.GetFqdn())
}

func (h *GSLBHandler) ServeDNS(w dns.ResponseWriter, msg *dns.Msg) {
	remoteAddr, _, err := net.SplitHostPort(w.RemoteAddr().String())
	if err != nil {
		log.Errorf("error parsing remote addr: %s", err)
	}
	ctx := contexes.SetRemoteAddr(context.Background(), remoteAddr)
	ctx = contexes.SetDNSMsg(ctx, msg)
	m := new(dns.Msg)
	m.SetReply(msg)
	m.Compress = true
	rrs := make([]dns.RR, 0)
	log.Debugf("receive request for with question: \n %s", msg.String())

	for _, question := range msg.Question {
		rrs = append(rrs, h.Resolve(ctx, question.Name, question.Qtype)...)
	}
	m.Answer = append(m.Answer, rrs...)

	// if in udp we check if we truncate to handle big answer and make dns client use tcp instead of udp to retrieve all
	if w.LocalAddr().Network() == "udp" {
		m.Truncate(dns.DefaultMsgSize)
	}
	err = w.WriteMsg(m)
	if err != nil {
		log.Errorf("error writing dns response: %s", err.Error())
	}
}

func (h *GSLBHandler) Resolve(ctx context.Context, fqdn string, queryType uint16) []dns.RR {
	seeAll := false

	if fqdn[:len(allMemberHost)] == allMemberHost {
		fqdn = fqdn[len(allMemberHost):]
		fmt.Println(fqdn)
		seeAll = true
	}
	entryRefRaw, ok := h.entries.Load(fqdn)
	if !ok {
		return []dns.RR{}
	}

	queryTypeStr, ok := dns.TypeToString[queryType]
	if !ok {
		log.Errorf("query type is not supported")
		return []dns.RR{}
	}

	er := entryRefRaw.(entryRef)
	var memberType lb.MemberType
	switch queryType {
	case dns.TypeTXT:
		if !h.isAllowedInspect(ctx) {
			return []dns.RR{}
		}
		return h.answerJson(er.entry)
	case dns.TypeA:
		memberType = lb.Ipv4
	case dns.TypeAAAA:
		memberType = lb.Ipv6
	case dns.TypeANY:
		memberType = lb.All
	default:
		return []dns.RR{}
	}
	ttl := defaultTtl
	if er.entry.GetTtl() > 0 {
		ttl = int(er.entry.GetTtl())
	}
	if seeAll && h.isAllowedInspect(ctx) {
		return h.seeAll(er.entry, queryType)
	}
	members, err := h.findMembers(ctx, er, memberType)
	if err != nil {
		log.Errorf("error finding members: %s", err.Error())
		return []dns.RR{}
	}
	rrs := make([]dns.RR, 0)
	for _, member := range members {
		rr, err := dns.NewRR(
			fmt.Sprintf("%s %d IN %s %s", fqdn, ttl, queryTypeStr, member.GetIp()),
		)
		if err != nil {
			log.Errorf("error creating dns RR: %s", err.Error())
			continue
		}
		rrs = append(rrs, rr)
	}

	return rrs
}

func (h *GSLBHandler) isAllowedInspect(ctx context.Context) bool {
	remoteAddr := net.ParseIP(contexes.GetRemoteAddr(ctx))
	for _, cidr := range h.allowedInspect {
		if cidr.IpNet.Contains(remoteAddr) {
			return true
		}
	}
	return false
}

func (h *GSLBHandler) answerJson(entry *entries.Entry) []dns.RR {
	var b []byte
	// could not happen error here
	b, _ = protojson.Marshal(entry) // nolint: errcheck

	rr, err := dns.NewRR(
		fmt.Sprintf("%s IN TXT %s", entry.GetFqdn(), base64.StdEncoding.EncodeToString(b)),
	)
	if err != nil {
		log.Errorf("error creating dns RR: %s", err.Error())
		return []dns.RR{}
	}
	return []dns.RR{rr}
}

func (h *GSLBHandler) seeAll(entry *entries.Entry, queryType uint16) []dns.RR {
	var members []*entries.Member
	switch queryType {
	case dns.TypeA:
		members = entry.GetMembersIpv4()
	case dns.TypeAAAA:
		members = entry.GetMembersIpv6()
	case dns.TypeANY:
		members = append(entry.GetMembersIpv4(), entry.GetMembersIpv6()...)
	default:
		return []dns.RR{}
	}
	rrs := make([]dns.RR, 0)
	for _, member := range members {
		rr, err := dns.NewRR(
			fmt.Sprintf("%s %d IN %s %s", entry.GetFqdn(), entry.GetTtl(), dns.TypeToString[queryType], member.GetIp()),
		)
		if err != nil {
			log.Errorf("error creating dns RR: %s", err.Error())
			continue
		}
		rrs = append(rrs, rr)
	}
	return rrs
}

func (h *GSLBHandler) findMembers(ctx context.Context, er entryRef, memberType lb.MemberType) ([]*entries.Member, error) {
	members := make([]*entries.Member, 0)
	if er.entry.GetMaxAnswerReturned() <= 1 {
		member, err := h.findMember(ctx, er, memberType, nil)
		if err != nil {
			log.Errorf("error finding member: %s", err.Error())
			return nil, err
		}
		members = append(members, member)
		return members, nil
	}

	// avoid duplicate using map
	membersMap := make(map[string]*entries.Member)
	for i := 0; i < int(er.entry.GetMaxAnswerReturned()); i++ {
		member, err := h.findMember(ctx, er, memberType, nil)
		if err != nil {
			log.Errorf("error finding member: %s", err.Error())
			return nil, err
		}
		membersMap[member.GetIp()] = member
	}

	for _, member := range membersMap {
		members = append(members, member)
	}
	return members, nil
}

func (h *GSLBHandler) findMember(ctx context.Context, er entryRef, memberType lb.MemberType, prevErr error) (*entries.Member, error) {
	lbler := er.lbPreferred
	if prevErr != nil {
		lbler = er.lbAlternate
	}
	nextMember, err := lbler.Next(ctx, memberType)
	if err == nil {
		return nextMember, nil
	}
	if prevErr == nil {
		return h.findMember(ctx, er, memberType, err)
	}

	result := multierror.Append(prevErr, err)
	nextMember, err = er.lbFallback.Next(ctx, memberType)
	if err != nil {
		result = multierror.Append(result, err)
		return nil, fmt.Errorf("error finding member: %s", result.Error())
	}
	return nextMember, nil
}
