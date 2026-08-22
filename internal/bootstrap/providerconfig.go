package bootstrap

type ProviderConfig struct {
	Name   string
	APIKey string
}

func (c ProviderConfig) Enabled() bool {
	return c.APIKey != ""
}
