package bootstrap

type ProviderConfig struct {
	Name    string
	APIKey  string
	Model   string
	BaseURL string
}

func (c ProviderConfig) Enabled() bool {
	return c.APIKey != "" && c.Model != "" && c.BaseURL != ""
}
