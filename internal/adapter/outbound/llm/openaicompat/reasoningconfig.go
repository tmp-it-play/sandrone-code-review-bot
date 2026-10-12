package openaicompat

type reasoningConfig struct {
	Effort  string `json:"effort,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
	Exclude bool   `json:"exclude,omitempty"`
}
