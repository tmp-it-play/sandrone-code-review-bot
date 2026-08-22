package provider

import "github.com/it-play/sandrone-code-review-bot/internal/core/llm"

type Catalog struct {
	descriptors map[string]Descriptor
}

func NewCatalog() Catalog {
	return Catalog{descriptors: map[string]Descriptor{
		"gemini": {
			Name:       "gemini",
			Capability: llm.Capability{ToolCalling: true, JSONMode: true},
		},
		"groq": {
			Name:       "groq",
			Capability: llm.Capability{ToolCalling: true, JSONMode: true},
		},
		"openrouter": {
			Name:       "openrouter",
			Capability: llm.Capability{ToolCalling: true, JSONMode: true},
			Headers: map[string]string{
				"HTTP-Referer": "https://github.com/it-play/sandrone-code-review-bot",
				"X-Title":      "sandrone-code-review-bot",
			},
		},
		"nvidia": {
			Name:       "nvidia",
			Capability: llm.Capability{ToolCalling: true, JSONMode: true},
		},
	}}
}

func (c Catalog) Descriptor(name string) (Descriptor, bool) {
	descriptor, ok := c.descriptors[name]
	return descriptor, ok
}
