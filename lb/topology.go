package lb

import (
	"context"
	"fmt"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/entries/v1"
	"github.com/orange-cloudfoundry/gsloc/contexes"
	"github.com/orange-cloudfoundry/gsloc/geolocs"
	"github.com/samber/lo"
)

type Topology struct {
	geoLoc          *geolocs.GeoLoc
	entry           *entries.Entry
	membersDcAll    map[string][]*entries.Member
	membersDcIpv4   map[string][]*entries.Member
	membersDcIpv6   map[string][]*entries.Member
	possibleDcsAll  []string
	possibleDcsIpv4 []string
	possibleDcsIpv6 []string
}

func NewTopology(entry *entries.Entry, geoLoc *geolocs.GeoLoc) *Topology {
	return &Topology{
		geoLoc:          geoLoc,
		entry:           entry,
		possibleDcsAll:  extractDc(append(entry.GetMembersIpv4(), entry.GetMembersIpv6()...)),
		possibleDcsIpv4: extractDc(entry.GetMembersIpv4()),
		possibleDcsIpv6: extractDc(entry.GetMembersIpv6()),
		membersDcAll:    membersToMapDc(append(entry.GetMembersIpv4(), entry.GetMembersIpv6()...)),
		membersDcIpv4:   membersToMapDc(entry.GetMembersIpv4()),
		membersDcIpv6:   membersToMapDc(entry.GetMembersIpv6()),
	}
}

func (t *Topology) Next(ctx context.Context, memberType MemberType) (*entries.Member, error) {
	var possibleDcs []string
	var membersDc map[string][]*entries.Member
	switch memberType {
	case All:
		possibleDcs = t.possibleDcsAll
		membersDc = t.membersDcAll
	case Ipv6:
		possibleDcs = t.possibleDcsIpv6
		membersDc = t.membersDcIpv6
	default:
		possibleDcs = t.possibleDcsIpv4
		membersDc = t.membersDcIpv4
	}
	ip := t.findRemoteAddr(ctx)
	if ip == "" {
		return nil, fmt.Errorf("no remote address found")
	}
	dc, err := t.geoLoc.FindDc(ip, possibleDcs...)
	if err != nil {
		return nil, fmt.Errorf("unable to find dc for %s: %s", ip, err)
	}
	members := membersDc[dc]
	if len(members) == 0 {
		return nil, fmt.Errorf("no member found for dc %s", dc)
	}
	if len(members) == 1 {
		return members[0], nil
	}
	return lo.Sample[*entries.Member](members), nil
}

func (t *Topology) findRemoteAddr(ctx context.Context) string {
	return contexes.GetRemoteAddr(ctx)
}

func (t *Topology) Reset() error {
	return nil
}

func (t *Topology) Name() string {
	return "topology"
}

func membersToMapDc(members []*entries.Member) map[string][]*entries.Member {
	membersMap := make(map[string][]*entries.Member)
	for _, m := range members {
		if _, ok := membersMap[m.GetDc()]; !ok {
			membersMap[m.GetDc()] = []*entries.Member{}
		}
		membersMap[m.GetDc()] = append(membersMap[m.GetDc()], m)
	}
	return membersMap
}

func extractDc(members []*entries.Member) []string {
	dcsMap := make(map[string]struct{})
	for _, m := range members {
		dcsMap[m.GetDc()] = struct{}{}
	}
	var dcs []string
	for dc := range dcsMap {
		dcs = append(dcs, dc)
	}
	return dcs
}
