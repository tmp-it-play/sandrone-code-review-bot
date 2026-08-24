package bootstrap

type ProviderConfig struct {
	Name               string
	Model              string
	APIKey             string
	BaseURL            string
	PrivateCodeAllowed bool
}

func (c ProviderConfig) Enabled() bool {
	return c.Model != "" && c.APIKey != "" && c.BaseURL != ""
}
