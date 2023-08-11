package disco

import (
	"fmt"
	consul "github.com/hashicorp/consul/api"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/entries/v1"
	"github.com/orange-cloudfoundry/gsloc/config"
	"github.com/orange-cloudfoundry/gsloc/observe"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/protojson"
)

type ConsulDiscoverer struct {
	consulClient *consul.Client
	hcAddr       string
	dcName       string
}

func NewConsulDiscoverer(consulClient *consul.Client, dcName, hcAddr string) *ConsulDiscoverer {
	return &ConsulDiscoverer{
		consulClient: consulClient,
		hcAddr:       hcAddr,
		dcName:       dcName,
	}
}

func (cd *ConsulDiscoverer) SetKVEntry(entry *entries.SignedEntry) {
	cd.registerMembers(entry, entry.GetEntry().GetMembersIpv4())
	cd.registerMembers(entry, entry.GetEntry().GetMembersIpv6())
}
func (cd *ConsulDiscoverer) registerMembers(entry *entries.SignedEntry, members []*entries.Member) {
	hcBytes, err := protojson.Marshal(entry.GetHealthcheck())
	if err != nil {
		log.Errorf("unable to marshal healthcheck: %v", err)
		return
	}
	parentTags := make([]string, 0)
	if len(entry.GetEntry().GetTags()) > 0 {
		for _, tag := range entry.GetEntry().GetTags() {
			parentTags = append(parentTags, fmt.Sprintf("%s%s", config.ConsulPrefixTagTag, tag))
		}
	}
	for _, member := range members {
		if member.GetDc() != cd.dcName {
			continue
		}
		observe.EmitMember(observe.EventTypeSet, &observe.MemberFqdn{
			Fqdn:   entry.GetEntry().GetFqdn(),
			Member: member,
		})
		id := fmt.Sprintf("%s%s", entry.GetEntry().GetFqdn(), member.GetIp())
		tags := append(parentTags, []string{
			fmt.Sprintf("%s%d", config.ConsulPrefixTagRatio, member.GetRatio()),
			fmt.Sprintf("%s%s", config.ConsulPrefixTagDc, member.GetDc()),
		}...)
		if member.GetDisabled() {
			tags = append(tags, config.ConsulPrefixTagDisabled)
		}
		err := cd.consulClient.Agent().ServiceRegister(&consul.AgentServiceRegistration{
			ID:   id,
			Name: entry.GetEntry().GetFqdn(),
			Tags: tags,
			Meta: map[string]string{
				config.ConsulMetaDcKey:    member.GetDc(),
				config.ConsulMetaEntryKey: "true",
			},
			Address: member.GetIp(),
			Check: &consul.AgentServiceCheck{
				Interval:      entry.GetHealthcheck().GetInterval().AsDuration().String(),
				Timeout:       entry.GetHealthcheck().GetTimeout().AsDuration().String(),
				TLSSkipVerify: true,
				HTTP:          fmt.Sprintf("https://%s/hc/%s/member/%s", cd.hcAddr, entry.GetEntry().GetFqdn(), member.GetIp()),
				Method:        "POST",
				Body:          string(hcBytes),
			},
		})
		if err != nil {
			log.WithError(err).Warning("Failed to register service")
		}
	}
}

func (cd *ConsulDiscoverer) deregisterMembers(entry *entries.SignedEntry, members []*entries.Member) {
	for _, member := range members {
		id := fmt.Sprintf("%s%s", entry.GetEntry().GetFqdn(), member.GetIp())
		err := cd.consulClient.Agent().ServiceDeregister(id)
		if err != nil {
			log.WithError(err).Warning("Failed to deregister service")
		}
	}
}

func (cd *ConsulDiscoverer) RemoveKvEntry(entry *entries.SignedEntry) {
	cd.deregisterMembers(entry, entry.GetEntry().GetMembersIpv4())
	cd.deregisterMembers(entry, entry.GetEntry().GetMembersIpv6())
}
