package config

type HealthCheckConfig struct {
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
	CA                 string `yaml:"ca"`
	HealthcheckAddress string `yaml:"healthcheck_address"`
}

func (c *HealthCheckConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain HealthCheckConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	return nil
}
