package bootstrap

type ProviderConfig struct {
	Name               string
	APIKey             string
	BaseURL            string
	PrivateCodeAllowed bool
}

func (c ProviderConfig) Enabled() bool {
	return c.APIKey != "" && c.BaseURL != ""
}
