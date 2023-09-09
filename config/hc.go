package config

type HealthCheckConfig struct {
	HealthcheckAddress string     `yaml:"healthcheck_address"`
	HealthcheckAuth    *BasicAuth `yaml:"healthcheck_auth"`
	AllowOnlyLocalhost bool       `yaml:"allow_only_localhost"`
}

func (c *HealthCheckConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain HealthCheckConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	return nil
}
