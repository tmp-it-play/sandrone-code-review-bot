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
		requestProfile := provider.RequestProfileFor(name, entry.Model)
		capability := descriptor.Capability
		if requestProfile.Capability != nil {
			capability = *requestProfile.Capability
		}
		providers = append(providers, openaicompat.NewClient(
			name,
			entry.Model,
			entry.BaseURL,
			entry.APIKey,
			capability,
			requestProfile,
			descriptor.MaxPromptChars,
			descriptor.Headers,
			config.RequestTimeout,
		))
	}
	return providers
}
