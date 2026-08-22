package openaicompat

type reasoningConfig struct {
	Effort  string `json:"effort"`
	Exclude bool   `json:"exclude,omitempty"`
}
