package lb

import (
	"context"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/entries/v1"
	"math/rand"
)

type ReturnToDns struct {
	entry      *entries.Entry
	allMembers []*entries.Member
}

func NewReturnToDns(entry *entries.Entry) *ReturnToDns {
	return &ReturnToDns{
		entry:      entry,
		allMembers: append(entry.GetMembersIpv4(), entry.GetMembersIpv6()...),
	}
}

func (rtd *ReturnToDns) Next(_ context.Context, memberType MemberType) (*entries.Member, error) {
	var members []*entries.Member
	switch memberType {
	case All:
		members = rtd.allMembers
	case Ipv6:
		members = rtd.entry.GetMembersIpv6()
	default:
		members = rtd.entry.GetMembersIpv4()
	}
	if len(members) == 0 {
		return nil, nil
	}
	return members[rand.Intn(len(members))], nil
}

func (rtd *ReturnToDns) Reset() error {
	return nil
}
