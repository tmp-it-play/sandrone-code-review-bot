package bootstrap

import (
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/llm/openaicompat"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/llm/provider"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

func buildProviders(config Config) []outbound.Provider {
	catalog := provider.NewCatalog()
	providers := make([]outbound.Provider, 0, len(config.ProviderOrder))
	for _, name := range config.ProviderOrder {
		entry, configured := config.Providers[name]
		if !configured {
			continue
		}
		descriptor, known := catalog.Descriptor(name)
		if !known {
			continue
		}
		requestProfile := provider.RequestProfileFor(descriptor.Name, entry.Model)
		capability := descriptor.Capability
		if override := requestProfile.Capability; override != nil {
			capability = *override
		}
		profile := descriptor.Profile
		profile.PrivateCodeAllowed = entry.PrivateCodeAllowed
		providers = append(providers, openaicompat.NewClient(
			descriptor.Name,
			entry.Model,
			descriptor.DisplayName,
			entry.BaseURL,
			entry.APIKey,
			capability,
			profile,
			requestProfile,
			descriptor.MaxPromptChars,
			descriptor.MaxConcurrency,
			descriptor.Headers,
			config.RequestTimeout,
		))
	}
	return providers
}
