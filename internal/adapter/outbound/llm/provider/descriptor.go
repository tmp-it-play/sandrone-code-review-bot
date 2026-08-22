package provider

import "github.com/it-play/sandrone-code-review-bot/internal/core/llm"

type Descriptor struct {
	Name       string
	Capability llm.Capability
	Headers    map[string]string
}
