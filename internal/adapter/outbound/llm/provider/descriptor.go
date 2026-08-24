package provider

import (
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
)

type Descriptor struct {
	Name           string
	Model          string
	DisplayName    string
	BaseURL        string
	APIKeyEnv      string
	BaseURLEnv     string
	ModelEnv       string
	AccountIDEnv   string
	Optional       bool
	Capability     llm.Capability
	Profile        llm.ProviderProfile
	MaxPromptChars int
	MaxConcurrency int
	Headers        map[string]string
}

func (d Descriptor) ResolvedBaseURL(accountID string) string {
	if d.AccountIDEnv == "" {
		return d.BaseURL
	}
	return strings.ReplaceAll(d.BaseURL, "{account_id}", strings.TrimSpace(accountID))
}
