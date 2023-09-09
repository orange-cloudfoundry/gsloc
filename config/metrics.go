package config

import (
	"fmt"
	"regexp"
)

var validateNameForPath = regexp.MustCompile(`(\s|_)`).MatchString

type MetricsConfig struct {
	ProxyMetricsConfig *ProxyMetricsConfig `yaml:"proxy"`
}

func (u *MetricsConfig) init() error {
	if u.ProxyMetricsConfig == nil {
		u.ProxyMetricsConfig = &ProxyMetricsConfig{}
		err := u.ProxyMetricsConfig.init()
		if err != nil {
			return err
		}
	}
	return nil
}

type ProxyMetricsConfig struct {
	Targets []*ProxyMetricsTarget `yaml:"targets"`
}

func (u *ProxyMetricsConfig) init() error {
	if u.Targets == nil {
		u.Targets = make([]*ProxyMetricsTarget, 0)
	}
	return nil
}

type ProxyMetricsTarget struct {
	Name string    `yaml:"name"`
	URL  URLConfig `yaml:"url"`
}

func (u *ProxyMetricsTarget) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain ProxyMetricsTarget
	err := unmarshal((*plain)(u))
	if err != nil {
		return err
	}
	if u.Name == "" {
		return fmt.Errorf("proxy metrics target name is empty")
	}
	if validateNameForPath(u.Name) {
		return fmt.Errorf("proxy metrics target name must not contains whitespace or underscore")
	}
	if u.URL.Raw == "" {
		return fmt.Errorf("proxy metrics target url is empty")
	}

	return nil
}
