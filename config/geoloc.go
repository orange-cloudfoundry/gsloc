package config

import (
	"fmt"
	"github.com/oschwald/geoip2-golang"
	"net"
	"net/url"
)

type GeoLoc struct {
	DcPositions []*DcPosition `yaml:"dc_positions"`
	GeoDb       *GeoDb        `yaml:"geo_db"`
}

type GeoDb struct {
	Path   string         `yaml:"path"`
	Reader *geoip2.Reader `yaml:"-"`
}

func (g *GeoDb) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain GeoDb
	err := unmarshal((*plain)(g))
	if err != nil {
		return err
	}
	if g.Path == "" {
		return fmt.Errorf("db_path is empty")
	}
	var errDb error
	g.Reader, errDb = geoip2.Open(g.Path)
	if errDb != nil {
		return errDb
	}
	return nil
}

func (g *GeoLoc) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain GeoLoc
	err := unmarshal((*plain)(g))
	if err != nil {
		return err
	}
	if len(g.DcPositions) == 0 {
		return fmt.Errorf("dc_positions is empty")
	}

	return nil
}

type Position struct {
	Longitude float64 `yaml:"longitude"`
	Latitude  float64 `yaml:"latitude"`
}

type DcPosition struct {
	DcName   string   `yaml:"dc_name"`
	Position Position `yaml:"position"`
	Cidrs    []*CIDR  `yaml:"cidrs"`
}

func (c *DcPosition) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain DcPosition
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.DcName == "" {
		return fmt.Errorf("dc name is empty")
	}
	return nil
}

type CIDR struct {
	IpNet *net.IPNet
}

func (c *CIDR) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var rawCIDR string
	err := unmarshal(&rawCIDR)
	if err != nil {
		return err
	}
	if rawCIDR == "" {
		return fmt.Errorf("cidr is empty")
	}
	_, ipNet, err := net.ParseCIDR(rawCIDR)
	if err != nil {
		return err
	}
	c.IpNet = ipNet
	return nil
}

type URLConfig struct {
	URL *url.URL
	Raw string
}

func (uc *URLConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	err := unmarshal(&s)
	if err != nil {
		return err
	}
	uc.Raw = s
	uc.URL, err = url.Parse(s)
	return err
}
