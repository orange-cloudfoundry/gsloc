package config

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"net"
	"os"
	"time"
)

type DNSServerConfig struct {
	Listen         string  `yaml:"listen"`
	TrustEdns      bool    `yaml:"trust_edns"`
	AllowedInspect []*CIDR `yaml:"allowed_inspect"`
}

func (c *DNSServerConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain DNSServerConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.Listen == "" {
		c.Listen = "0.0.0.0:53"
	}
	return nil
}

type Config struct {
	DNSServer         *DNSServerConfig   `yaml:"dns_server"`
	HTTPServer        *HTTPServerConfig  `yaml:"http_server"`
	Log               *Log               `yaml:"log"`
	DcName            string             `yaml:"dc_name"`
	ConsulConfig      *ConsulConfig      `yaml:"consul_config"`
	HealthCheckConfig *HealthCheckConfig `yaml:"healthcheck_config"`
	GeoLoc            *GeoLoc            `yaml:"geo_loc"`
}

func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Config
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.DNSServer == nil {
		c.DNSServer = &DNSServerConfig{
			Listen: "0.0.0.0:53",
		}
	}
	if c.HTTPServer == nil {
		c.HTTPServer = &HTTPServerConfig{
			Listen: "0.0.0.0:8080",
		}
	}
	if c.HealthCheckConfig == nil {
		c.HealthCheckConfig = &HealthCheckConfig{}
	}
	if c.HealthCheckConfig.HealthcheckAddress == "" {
		_, port, err := net.SplitHostPort(c.HTTPServer.Listen)
		if err != nil {
			return fmt.Errorf("split host port: %w", err)
		}
		c.HealthCheckConfig.HealthcheckAddress = fmt.Sprintf("127.0.0.1:%s", port)
	}
	if c.ConsulConfig == nil {
		return fmt.Errorf("consul_config is required")
	}
	if c.DcName == "" {
		return fmt.Errorf("dc_name is required")
	}
	if c.GeoLoc == nil {
		return fmt.Errorf("geo_loc is required")
	}
	return nil
}

type HTTPServerConfig struct {
	Listen string `yaml:"listen"`
	TLSPem TLSPem `yaml:"tls_pem"`
}

func (c *HTTPServerConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain HTTPServerConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.Listen == "" {
		c.Listen = "0.0.0.0:8443"
	}
	if c.TLSPem.CertPath == "" {
		return fmt.Errorf("tls_pem.cert_path is required")
	}
	if c.TLSPem.PrivateKeyPath == "" {
		return fmt.Errorf("tls_pem.private_key_path is required")
	}
	return nil
}

type TLSPem struct {
	CertPath       string `yaml:"cert_path"`
	PrivateKeyPath string `yaml:"private_key_path"`
}

type ConsulConfig struct {
	Addr          string    `yaml:"addr"`
	Scheme        string    `yaml:"scheme"`
	Token         string    `yaml:"token"`
	Username      string    `yaml:"username"`
	Password      string    `yaml:"password"`
	ScrapInterval *Duration `yaml:"scrap_interval"`
}

func (c *ConsulConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain ConsulConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.Addr == "" {
		c.Addr = "127.0.0.1:5800"
	}
	if c.Scheme == "" {
		c.Scheme = "http"
	}
	if c.ScrapInterval == nil || *c.ScrapInterval <= 0 {
		dur := Duration(time.Second * 30)
		c.ScrapInterval = &dur
	}
	return nil
}

type Duration time.Duration

func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	err := unmarshal(&s)
	if err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(dur)
	return nil
}

type Log struct {
	Level   string `yaml:"level"`
	NoColor bool   `yaml:"no_color"`
	InJson  bool   `yaml:"in_json"`
}

func (c *Log) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Log
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	log.SetFormatter(&log.TextFormatter{
		DisableColors: c.NoColor,
	})
	if c.Level != "" {
		lvl, err := log.ParseLevel(c.Level)
		if err != nil {
			return err
		}
		log.SetLevel(lvl)
	}
	if c.InJson {
		log.SetFormatter(&log.JSONFormatter{})
	}

	return nil
}

func LoadConfig(path string) (*Config, error) {
	cnf := &Config{}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(b, cnf)
	if err != nil {
		return nil, err
	}
	return cnf, nil
}
