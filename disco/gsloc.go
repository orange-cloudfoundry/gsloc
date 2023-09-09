package disco

import (
	consul "github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-multierror"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/entries/v1"
	gslbsvc "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/services/gslb/v1"
	"github.com/orange-cloudfoundry/gsloc/config"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"
	"github.com/sourcegraph/conc/pool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"strings"
	"sync"
)

type GslocConsul struct {
	consulClient *consul.Client
}

func NewGslocConsul(consulClient *consul.Client) *GslocConsul {
	return &GslocConsul{
		consulClient: consulClient,
	}
}

func (c *GslocConsul) ConvertPairToSignedEntry(pair *consul.KVPair) (*entries.SignedEntry, error) {
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

func (c *GslocConsul) ListEntries(prefix string, tags []string) ([]*entries.SignedEntry, error) {
	pairs, _, err := c.consulClient.KV().List(config.ConsulKVEntriesPrefix+prefix, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list entries: %v", err)
	}

	ents := make([]*entries.SignedEntry, 0, len(pairs))
	for _, pair := range pairs {
		signedEntry, err := c.ConvertPairToSignedEntry(pair)
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

func (c *GslocConsul) ListEntriesStatus(prefix string, tags []string) ([]*gslbsvc.GetEntryStatusResponse, error) {
	ents, err := c.ListEntries(prefix, tags)
	if err != nil {
		return nil, err
	}
	var entsStatus []*gslbsvc.GetEntryStatusResponse
	var chanEntsStatus = make(chan *gslbsvc.GetEntryStatusResponse, 100)
	p := pool.New().WithMaxGoroutines(10)
	done := make(chan struct{})
	go func() {
		for entStatus := range chanEntsStatus {
			entsStatus = append(entsStatus, entStatus)
		}
		done <- struct{}{}
	}()
	lockErr := &sync.Mutex{}
	var errResult error
	for _, ent := range ents {
		ent := ent
		p.Go(func() {
			entStatus, err := c.GetEntryStatus(ent.GetEntry().GetFqdn())
			if err != nil {
				lockErr.Lock()
				errResult = multierror.Append(errResult, err)
				lockErr.Unlock()
			}
			chanEntsStatus <- entStatus
		})
	}
	p.Wait()
	close(chanEntsStatus)
	<-done
	return entsStatus, errResult
}

func (c *GslocConsul) GetEntryStatus(fqdn string) (*gslbsvc.GetEntryStatusResponse, error) {
	pair, _, err := c.consulClient.KV().Get(config.ConsulKVEntriesPrefix+fqdn, nil)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "entry not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get entry: %v", err)
	}

	signedEntry, err := c.ConvertPairToSignedEntry(pair)
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

	ents, _, err := c.consulClient.Health().Service(fqdn, "", false, &consul.QueryOptions{})
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

func (c *GslocConsul) RetrieveSignedEntry(fqdn string) (*entries.SignedEntry, error) {
	pair, _, err := c.consulClient.KV().Get(config.ConsulKVEntriesPrefix+fqdn, nil)
	if err != nil {
		return nil, err
	}
	signedEntry, err := c.ConvertPairToSignedEntry(pair)
	if err != nil {
		return nil, err
	}
	return signedEntry, nil
}
