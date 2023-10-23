package config

import (
	"fmt"
	"os/exec"
)

type HealthCheckConfig struct {
	HealthcheckAddress string                     `yaml:"healthcheck_address"`
	HealthcheckAuth    *BasicAuth                 `yaml:"healthcheck_auth"`
	AllowOnlyLocalhost bool                       `yaml:"allow_only_localhost"`
	Plugins            []*PluginHealthCheckConfig `yaml:"plugins"`
}

func (c *HealthCheckConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain HealthCheckConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	return nil
}

type PluginHealthCheckConfig struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Path        string   `yaml:"path"`
	Args        []string `yaml:"args"`
}

func (c *PluginHealthCheckConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain PluginHealthCheckConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.Name == "" {
		return fmt.Errorf("missing name in plugin")
	}
	if c.Description == "" {
		return fmt.Errorf("missing description for plugin %s", c.Name)
	}
	if c.Path == "" {
		return fmt.Errorf("missing path for plugin %s", c.Name)
	}
	_, err = exec.LookPath(c.Path)
	if err != nil {
		return fmt.Errorf("unable to find path %s for plugin %s", c.Path, c.Name)
	}
	return nil
}
