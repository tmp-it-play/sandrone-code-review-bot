package provider

import "github.com/it-play/sandrone-code-review-bot/internal/core/llm"

type Catalog struct {
	descriptors map[string]Descriptor
	order       []string
}

func NewCatalog() Catalog {
	verifiedRoles := supportedRoles()
	promptContractRoles := supportedPromptContractRoles()
	return Catalog{
		order: []string{"gemini", "nvidia", "openrouter", "groq", "mistral", "cloudflare-glm", "cloudflare-gemma"},
		descriptors: map[string]Descriptor{
			"gemini": {
				Name:           "gemini",
				Model:          "gemini-3.7-flash",
				DisplayName:    "Gemini 3.7 Flash",
				BaseURL:        "https://generativelanguage.googleapis.com/v1beta/openai",
				Capability:     llm.Capability{ToolCalling: true, JSONMode: true},
				Profile:        llm.ProviderProfile{Roles: verifiedRoles, PublicDataAllowed: true},
				MaxPromptChars: 600000,
			},
			"groq": {
				Name:           "groq",
				Model:          "openai/gpt-oss-120b",
				DisplayName:    "GPT-OSS 120B",
				BaseURL:        "https://api.groq.com/openai/v1",
				Capability:     llm.Capability{ToolCalling: true, JSONMode: true},
				Profile:        llm.ProviderProfile{Roles: verifiedRoles, PublicDataAllowed: true},
				MaxPromptChars: 12000,
			},
			"openrouter": {
				Name:           "openrouter",
				Model:          "z-ai/glm-5.2:free",
				DisplayName:    "GLM 5.2",
				BaseURL:        "https://openrouter.ai/api/v1",
				Capability:     llm.Capability{ToolCalling: true, JSONMode: true},
				Profile:        llm.ProviderProfile{Roles: verifiedRoles, PublicDataAllowed: true},
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
				Profile:        llm.ProviderProfile{Roles: promptContractRoles, PublicDataAllowed: true},
				MaxPromptChars: 180000,
			},
			"mistral": {
				Name:           "mistral",
				Model:          "mistral-small-2603",
				DisplayName:    "Mistral Small 4",
				BaseURL:        "https://api.mistral.ai/v1",
				Capability:     llm.Capability{ToolCalling: true, JSONMode: true},
				Profile:        llm.ProviderProfile{Roles: verifiedRoles, PublicDataAllowed: true},
				MaxPromptChars: 600000,
			},
			"cloudflare-glm": {
				Name:           "cloudflare-glm",
				Model:          "@cf/zai-org/glm-4.7-flash",
				DisplayName:    "Cloudflare GLM 4.7 Flash",
				BaseURL:        "https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1",
				AccountIDEnv:   "CLOUDFLARE_ACCOUNT_ID",
				Capability:     llm.Capability{ToolCalling: true},
				Profile:        llm.ProviderProfile{Roles: promptContractRoles, PublicDataAllowed: true},
				MaxPromptChars: 450000,
			},
			"cloudflare-gemma": {
				Name:           "cloudflare-gemma",
				Model:          "@cf/google/gemma-4-26b-a4b-it",
				DisplayName:    "Cloudflare Gemma 4 26B",
				BaseURL:        "https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1",
				AccountIDEnv:   "CLOUDFLARE_ACCOUNT_ID",
				Capability:     llm.Capability{ToolCalling: true},
				Profile:        llm.ProviderProfile{Roles: promptContractRoles, PublicDataAllowed: true},
				MaxPromptChars: 450000,
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
	case "mistral":
		return "MISTRAL_API_KEY"
	case "cloudflare-glm", "cloudflare-gemma":
		return "CLOUDFLARE_API_TOKEN"
	default:
		return ""
	}
}

func supportedRoles() []llm.TaskRole {
	return []llm.TaskRole{
		llm.TaskRolePlanner,
		llm.TaskRoleReviewer,
		llm.TaskRoleVerifier,
		llm.TaskRoleReducer,
		llm.TaskRoleSummary,
		llm.TaskRoleReply,
	}
}

func supportedPromptContractRoles() []llm.TaskRole {
	return []llm.TaskRole{
		llm.TaskRoleReviewer,
		llm.TaskRoleSummary,
		llm.TaskRoleReply,
	}
}
