package healthchecks

type NoHealthCheck struct {
}

func NewNoHealthCheck() *NoHealthCheck {
	return &NoHealthCheck{}
}

func (h *NoHealthCheck) Check(host string) error {
	return nil
}
