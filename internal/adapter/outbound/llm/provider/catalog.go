package provider

import "github.com/it-play/sandrone-code-review-bot/internal/core/llm"

type Catalog struct {
	descriptors map[string]Descriptor
	order       []string
}

func NewCatalog() Catalog {
	return Catalog{
		order: []string{"gemini", "groq", "openrouter", "nvidia"},
		descriptors: map[string]Descriptor{
			"gemini": {
				Name:           "gemini",
				Model:          "gemini-3.7-flash",
				DisplayName:    "Gemini 3.7 Flash",
				BaseURL:        "https://generativelanguage.googleapis.com/v1beta/openai",
				Capability:     llm.Capability{ToolCalling: true, JSONMode: true},
				MaxPromptChars: 600000,
			},
			"groq": {
				Name:           "groq",
				Model:          "openai/gpt-oss-120b",
				DisplayName:    "GPT-OSS 120B",
				BaseURL:        "https://api.groq.com/openai/v1",
				Capability:     llm.Capability{ToolCalling: true, JSONMode: true},
				MaxPromptChars: 60000,
			},
			"openrouter": {
				Name:           "openrouter",
				Model:          "z-ai/glm-5.2:free",
				DisplayName:    "GLM 5.2",
				BaseURL:        "https://openrouter.ai/api/v1",
				Capability:     llm.Capability{ToolCalling: true, JSONMode: true},
				MaxPromptChars: 180000,
				Headers: map[string]string{
					"HTTP-Referer": "https://github.com/it-play/sandrone-code-review-bot",
					"X-Title":      "sandrone-code-review-bot",
				},
			},
			"nvidia": {
				Name:           "nvidia",
				Model:          "google/gemma-4-31b-it",
				DisplayName:    "Gemma 4 31B",
				BaseURL:        "https://integrate.api.nvidia.com/v1",
				Capability:     llm.Capability{ToolCalling: true, JSONMode: true},
				MaxPromptChars: 180000,
			},
		},
	}
}

func (c Catalog) Descriptor(name string) (Descriptor, bool) {
	descriptor, ok := c.descriptors[name]
	return descriptor, ok
}

func (c Catalog) Order() []string {
	order := make([]string, len(c.order))
	copy(order, c.order)
	return order
}

func (c Catalog) APIKeyEnv(name string) string {
	switch name {
	case "gemini":
		return "GEMINI_API_KEY"
	case "groq":
		return "GROQ_API_KEY"
	case "openrouter":
		return "OPENROUTER_API_KEY"
	case "nvidia":
		return "NVIDIA_API_KEY"
	default:
		return ""
	}
}
