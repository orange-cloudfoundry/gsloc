package lb

import (
	"context"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/entries/v1"
	"sync/atomic"
)

type RoundRobin struct {
	numberAll  *atomic.Int32
	numberIpv4 *atomic.Int32
	numberIpv6 *atomic.Int32
	entry      *entries.Entry
	allMembers []*entries.Member
}

func NewRoundRobin(entry *entries.Entry) *RoundRobin {
	rr := &RoundRobin{
		numberAll:  new(atomic.Int32),
		numberIpv4: new(atomic.Int32),
		numberIpv6: new(atomic.Int32),
		entry:      entry,
		allMembers: append(entry.GetMembersIpv4(), entry.GetMembersIpv6()...),
	}
	rr.numberAll.Store(-1)
	rr.numberIpv4.Store(-1)
	rr.numberIpv6.Store(-1)
	return rr
}

func (rr *RoundRobin) nextMember(members []*entries.Member, nb *atomic.Int32) (*entries.Member, error) {
	if len(members) == 0 {
		return nil, nil
	}
	number := nb.Add(1)
	return members[number%int32(len(members))], nil
}

func (rr *RoundRobin) Next(_ context.Context, memberType MemberType) (*entries.Member, error) {
	var members []*entries.Member
	var number *atomic.Int32
	switch memberType {
	case All:
		members = rr.allMembers
		number = rr.numberAll
	case Ipv6:
		members = rr.entry.GetMembersIpv6()
		number = rr.numberIpv6
	default:
		members = rr.entry.GetMembersIpv4()
		number = rr.numberIpv4
	}
	return rr.nextMember(members, number)
}

func (rr *RoundRobin) Reset() error {
	rr.numberAll.Store(0)
	rr.numberIpv4.Store(0)
	rr.numberIpv6.Store(0)
	return nil
}

func (rr *RoundRobin) Name() string {
	return "round_robin"
}
