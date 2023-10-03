package lb

import (
	"context"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/entries/v1"
	"sync/atomic"
)

type weightedRef struct {
	gcd           int
	maxWeight     uint32
	currentWeight *atomic.Int32
	index         *atomic.Int32
	members       []*entries.Member
}

type WeightedRoundRobin struct {
	wrAll      *weightedRef
	wrIpv4     *weightedRef
	wrIpv6     *weightedRef
	entry      *entries.Entry
	allMembers []*entries.Member
}

func NewWeightedRoundRobin(entry *entries.Entry) *WeightedRoundRobin {
	wrr := &WeightedRoundRobin{
		entry:      entry,
		wrAll:      membersToWeightedRef(append(entry.GetMembersIpv4(), entry.GetMembersIpv6()...)),
		wrIpv4:     membersToWeightedRef(entry.GetMembersIpv4()),
		wrIpv6:     membersToWeightedRef(entry.GetMembersIpv6()),
		allMembers: append(entry.GetMembersIpv4(), entry.GetMembersIpv6()...),
	}
	return wrr
}
func (wrr *WeightedRoundRobin) nextMember(wr *weightedRef) (*entries.Member, error) {
	if len(wr.members) == 0 {
		return nil, nil
	}
	if len(wr.members) == 1 {
		return wr.members[0], nil
	}
	for {
		newIndex := (wr.index.Load() + 1) % int32(len(wr.members))
		wr.index.Store(newIndex)
		if newIndex == 0 {
			wr.currentWeight.Store(wr.currentWeight.Load() - int32(wr.gcd))
			if wr.currentWeight.Load() <= 0 {
				wr.currentWeight.Store(int32(wr.maxWeight))
				if wr.currentWeight.Load() == 0 {
					return nil, nil
				}
			}
		}
		if wr.members[newIndex].GetRatio() >= uint32(wr.currentWeight.Load()) {
			return wr.members[newIndex], nil
		}
	}
}

func (wrr *WeightedRoundRobin) Next(_ context.Context, memberType MemberType) (*entries.Member, error) {
	var wr *weightedRef
	switch memberType {
	case All:
		wr = wrr.wrAll
	case Ipv6:
		wr = wrr.wrIpv6
	default:
		wr = wrr.wrIpv4
	}
	return wrr.nextMember(wr)
}

func (wrr *WeightedRoundRobin) Reset() error {
	wrr.wrAll.index.Store(-1)
	wrr.wrAll.currentWeight.Store(0)
	wrr.wrIpv4.index.Store(-1)
	wrr.wrIpv4.currentWeight.Store(0)
	wrr.wrIpv6.index.Store(-1)
	wrr.wrIpv6.currentWeight.Store(0)
	return nil
}

func (wrr *WeightedRoundRobin) Name() string {
	return "weighted_round_robin"
}

func membersToWeightedRef(members []*entries.Member) *weightedRef {
	wr := &weightedRef{
		gcd:           0,
		maxWeight:     0,
		currentWeight: new(atomic.Int32),
		index:         new(atomic.Int32),
		members:       members,
	}
	wr.index.Store(-1)
	wr.currentWeight.Store(0)
	for _, member := range members {
		weight := member.GetRatio()
		if weight == 0 {
			weight = 1
		}
		if wr.gcd == 0 {
			wr.gcd = int(weight)
			wr.maxWeight = weight
		} else {
			wr.gcd = gcd(uint32(wr.gcd), weight)
		}
	}
	return wr
}

func gcd(x, y uint32) int {
	var t int
	for {
		t = int(x) % int(y)
		if t > 0 {
			x = y
			y = uint32(t)
		} else {
			return int(y)
		}
	}
}
