package rets

import (
	"context"
	"fmt"
	consul "github.com/hashicorp/consul/api"
	"github.com/miekg/dns"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/entries/v1"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/helpers"
	"github.com/orange-cloudfoundry/gsloc/config"
	"github.com/orange-cloudfoundry/gsloc/observe"
	log "github.com/sirupsen/logrus"
	"github.com/sourcegraph/conc/pool"
	"google.golang.org/protobuf/encoding/protojson"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Retriever struct {
	entry           *log.Entry
	consulClient    *consul.Client
	signEntsCached  *sync.Map
	signCheckCached *sync.Map
	dcName          string
	nbWorkers       int
	interval        time.Duration
}

func NewRetriever(dcName string, nbWorkers int, interval time.Duration, consulClient *consul.Client) *Retriever {
	return &Retriever{
		entry:           log.WithField("component", "retriever"),
		consulClient:    consulClient,
		signEntsCached:  &sync.Map{},
		signCheckCached: &sync.Map{},
		interval:        interval,
		dcName:          dcName,
		nbWorkers:       nbWorkers,
	}
}

func (r *Retriever) Run(ctx context.Context) error {
	r.entry.Info("starting retriever ...")
	err := r.pollKV()
	if err != nil {
		r.entry.WithError(err).Error("error while polling kv")
	}
	err = r.pollCatalog()
	if err != nil {
		r.entry.WithError(err).Error("error while polling catalog")
	}
	go func() {
		r.runKV(ctx)
	}()
	go func() {
		r.runCatalog(ctx)
	}()
	<-ctx.Done()
	r.entry.Info("retriever stopped")
	return nil
}

func (r *Retriever) runKV(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := r.pollKV()
			if err != nil {
				r.entry.WithError(err).Error("error while polling kv")
				continue
			}
			ticker.Reset(r.interval)
		}
	}
}

func (r *Retriever) runCatalog(ctx context.Context) {

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := r.pollCatalog()
			if err != nil {
				r.entry.WithError(err).Error("error while polling catalog")
				continue
			}
			ticker.Reset(r.interval)
		}
	}
}

func (r *Retriever) pollKV() error {
	r.entry.Info("polling kv ...")
	defer r.entry.Info("polling kv done.")
	kvPairs, _, err := r.consulClient.KV().List(config.ConsulKVEntriesPrefix, &consul.QueryOptions{})
	if err != nil {
		return fmt.Errorf("error while listing kv entries: %s", err)
	}
	log.Debugf("found %d kv entries", len(kvPairs))
	toRemove := map[string]struct{}{}
	r.signEntsCached.Range(func(key, value interface{}) bool {
		toRemove[key.(string)] = struct{}{}
		return true
	})
	p := pool.New().WithMaxGoroutines(r.nbWorkers)
	for _, kvPair := range kvPairs {
		kvPair := kvPair
		fqdn := dns.Fqdn(kvPair.Key[len(config.ConsulKVEntriesPrefix):])
		delete(toRemove, fqdn)
		p.Go(func() {
			signedEntry := &entries.SignedEntry{}
			err = protojson.Unmarshal(kvPair.Value, signedEntry)
			if err != nil {
				r.entry.WithError(err).Errorf("error while unmarshalling signed entry for %s", fqdn)
				return
			}
			rawEntry, loaded := r.signEntsCached.LoadOrStore(fqdn, signedEntry)
			if !loaded {
				log.Debugf("emitted signed entry for %s", fqdn)
				observe.EmitKvEntry(observe.EventTypeSet, signedEntry)
				return
			}
			actualEntry := rawEntry.(*entries.SignedEntry)
			if actualEntry.GetSignature() == signedEntry.GetSignature() {
				return
			}
			r.signEntsCached.Store(fqdn, signedEntry)
			log.Debugf("emitted signed entry for %s", fqdn)
			observe.EmitKvEntry(observe.EventTypeSet, signedEntry)
		})
	}
	p.Wait()
	for fqdn := range toRemove {
		rawKvEntry, ok := r.signEntsCached.Load(fqdn)
		entry := rawKvEntry.(*entries.SignedEntry)
		if ok {
			observe.EmitKvEntry(observe.EventTypeDelete, entry)
			r.signEntsCached.Delete(fqdn)
		}

		_, ok = r.signEntsCached.Load(fqdn)
		if ok {
			observe.EmitCatalogEntry(observe.EventTypeDelete, entry.Entry)
			r.signCheckCached.Delete(fqdn)
		}
	}
	return nil
}

func (r *Retriever) pollCatalog() error {
	r.entry.Info("polling catalog ...")
	defer r.entry.Info("polling catalog done.")

	svcs, _, err := r.consulClient.Catalog().Services(&consul.QueryOptions{
		Filter: fmt.Sprintf("ServiceMeta.%s == true and ServiceMeta.%s == %s",
			config.ConsulMetaEntryKey,
			config.ConsulMetaDcKey,
			r.dcName,
		),
	})
	if err != nil {
		return fmt.Errorf("error while listing catalog entries: %s", err)
	}
	log.Debugf("found %d catalog entries", len(svcs))
	p := pool.New().WithMaxGoroutines(r.nbWorkers)
	for svcName := range svcs {
		fqdn := svcName
		p.Go(func() {
			rawEntry, ok := r.signEntsCached.Load(fqdn)
			if !ok {
				return
			}
			currentEntry := rawEntry.(*entries.SignedEntry).GetEntry()

			signedEntry := &entries.SignedEntry{
				Entry: &entries.Entry{
					Fqdn:              currentEntry.GetFqdn(),
					LbAlgoPreferred:   currentEntry.GetLbAlgoPreferred(),
					LbAlgoAlternate:   currentEntry.GetLbAlgoAlternate(),
					LbAlgoFallback:    currentEntry.GetLbAlgoFallback(),
					MaxAnswerReturned: currentEntry.GetMaxAnswerReturned(),
					MembersIpv4:       nil,
					MembersIpv6:       nil,
					Ttl:               currentEntry.GetTtl(),
				},
			}

			ents, _, err := r.consulClient.Health().Service(fqdn, "", true, &consul.QueryOptions{})
			if err != nil {
				r.entry.WithError(err).Errorf("error while listing health entries for service %s", fqdn)
				return
			}
			membersIpv4 := make([]*entries.Member, 0)
			membersIpv6 := make([]*entries.Member, 0)
			for _, consulEnt := range ents {
				member := r.consulEntryToMember(consulEnt)
				if strings.Contains(consulEnt.Service.Address, ":") {
					membersIpv6 = append(membersIpv6, member)
					continue
				}
				membersIpv4 = append(membersIpv4, member)
			}
			signedEntry.Entry.MembersIpv4 = membersIpv4
			signedEntry.Entry.MembersIpv6 = membersIpv6

			newSig, err := helpers.MessageSignature(signedEntry)
			if err != nil {
				r.entry.WithError(err).Errorf("error while signing entry for %s", fqdn)
				return
			}

			signedEntry.Signature = newSig
			rawSign, loaded := r.signCheckCached.LoadOrStore(fqdn, newSig)
			if !loaded {
				log.Debugf("emitted catalog entry for %s", fqdn)
				observe.EmitCatalogEntry(observe.EventTypeSet, signedEntry.Entry)
				return
			}
			if rawSign.(string) == newSig {
				return
			}

			r.signCheckCached.Store(fqdn, newSig)
			log.Tracef("emitted catalog entry for %s (old sign: %s - new sign: %s )", fqdn, rawSign.(string), newSig)
			observe.EmitCatalogEntry(observe.EventTypeSet, signedEntry.Entry)
		})
	}
	p.Wait()
	return nil
}

func (r *Retriever) consulEntryToMember(consulEnt *consul.ServiceEntry) *entries.Member {
	ratio := 0
	dc := r.dcName
	disabled := false
	var err error
	for _, tag := range consulEnt.Service.Tags {
		if strings.HasPrefix(tag, config.ConsulPrefixTagRatio) {
			ratioStr := tag[len(config.ConsulPrefixTagRatio):]
			ratio, err = strconv.Atoi(ratioStr)
			if err != nil {
				ratio = 0
				r.entry.WithError(err).Errorf("error while parsing gsloc-ratio tag %s for %s", tag, consulEnt.Service.ID)
			}
			continue
		}
		if strings.HasPrefix(tag, config.ConsulPrefixTagDc) {
			dc = tag[len(config.ConsulPrefixTagDc):]
		}
		if strings.HasPrefix(tag, config.ConsulPrefixTagDisabled) {
			disabled = true
		}
	}
	return &entries.Member{
		Ip:       consulEnt.Service.Address,
		Ratio:    uint32(ratio),
		Dc:       dc,
		Disabled: disabled,
	}
}
