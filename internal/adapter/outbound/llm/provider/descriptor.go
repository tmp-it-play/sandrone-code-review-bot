package provider

import "github.com/it-play/sandrone-code-review-bot/internal/core/llm"

type Descriptor struct {
	Name           string
	Model          string
	DisplayName    string
	BaseURL        string
	Capability     llm.Capability
	MaxPromptChars int
	Headers        map[string]string
}
