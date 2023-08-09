package geolocs

import (
	"fmt"
	"github.com/orange-cloudfoundry/gsloc/config"
	"github.com/oschwald/geoip2-golang"
	"github.com/samber/lo"
	"math"
	"net"
	"sync"
)

type GeoLoc struct {
	dcPositions []*config.DcPosition
	geoDb       *geoip2.Reader
	cachedLoc   *sync.Map
}

func NewGeoLoc(dcPositions []*config.DcPosition, geoDb *geoip2.Reader) *GeoLoc {
	return &GeoLoc{
		dcPositions: dcPositions,
		geoDb:       geoDb,
		cachedLoc:   &sync.Map{},
	}
}

func (g *GeoLoc) FindDc(ip string, forDc ...string) (string, error) {
	dcName, ok := g.cachedLoc.Load(ip)
	if ok {
		return dcName.(string), nil
	}
	netIp := net.ParseIP(ip)
	for _, dcPos := range g.dcPositions {
		if len(forDc) > 0 && !lo.Contains[string](forDc, dcPos.DcName) {
			continue
		}
		for _, cidr := range dcPos.Cidrs {
			if cidr.IpNet.Contains(netIp) {
				g.cachedLoc.Store(ip, dcPos.DcName)
				return dcPos.DcName, nil
			}
		}
	}

	pos, err := g.findPosition(ip)
	if err != nil {
		return "", err
	}

	dcName = g.findNearest(pos, forDc...)
	if dcName == "" {
		return "", fmt.Errorf("no dc found for %s", ip)
	}
	g.cachedLoc.Store(ip, dcName)
	return dcName.(string), nil
}

func (g *GeoLoc) findNearest(pos config.Position, forDc ...string) string {
	minDistance := math.MaxFloat64
	var nearestDc string
	for _, dcPos := range g.dcPositions {
		if len(forDc) > 0 && !lo.Contains[string](forDc, dcPos.DcName) {
			continue
		}
		distance := distanceMeters(pos, dcPos.Position)
		if distance < minDistance {
			minDistance = distance
			nearestDc = dcPos.DcName
		}
	}
	return nearestDc
}

func (g *GeoLoc) findPosition(ip string) (config.Position, error) {
	record, err := g.geoDb.City(net.ParseIP(ip))
	if err != nil {
		return config.Position{}, err
	}
	return config.Position{
		Longitude: record.Location.Longitude,
		Latitude:  record.Location.Latitude,
	}, nil
}

func distanceMeters(firstPos, secondPos config.Position) float64 {
	x := deg2rad(firstPos.Longitude-secondPos.Longitude) * math.Cos(deg2rad((firstPos.Latitude+secondPos.Latitude)/2))
	y := deg2rad(firstPos.Latitude - secondPos.Latitude)
	return 6371000.0 * math.Sqrt(x*x+y*y)
}

func deg2rad(degrees float64) float64 {
	return float64(degrees) * (math.Pi / 180)
}
