package config

type HealthCheckConfig struct {
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
