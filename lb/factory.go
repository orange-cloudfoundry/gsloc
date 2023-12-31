package lb

import (
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/entries/v1"
	"github.com/orange-cloudfoundry/gsloc/geolocs"
)

type LBFactory struct {
	geoLoc    *geolocs.GeoLoc
	trustEdns bool
}

func NewLBFactory(geoLoc *geolocs.GeoLoc) *LBFactory {
	return &LBFactory{
		geoLoc: geoLoc,
	}
}

func (f *LBFactory) MakeLb(entry *entries.Entry, algo entries.LBAlgo) Loadbalancer {
	switch algo {
	case entries.LBAlgo_ROUND_ROBIN:
		return NewRoundRobin(entry)
	case entries.LBAlgo_RATIO:
		return NewWeightedRoundRobin(entry)
	case entries.LBAlgo_TOPOLOGY:
		return NewTopology(entry, f.geoLoc)
	case entries.LBAlgo_RANDOM:
		return NewRandom(entry)
	}
	return nil
}
