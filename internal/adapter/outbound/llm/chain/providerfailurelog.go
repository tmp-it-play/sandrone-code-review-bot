package chain

import "github.com/it-play/sandrone-code-review-bot/internal/core/llm"

func providerFailureLogFields(failure *llm.Failure) []any {
	if failure == nil {
		return nil
	}
	fields := make([]any, 0, 4)
	if failure.ProviderErrorCode != "" {
		fields = append(fields, "provider_error_code", failure.ProviderErrorCode)
	}
	if failure.Elapsed > 0 {
		fields = append(fields, "elapsed_ms", failure.Elapsed.Milliseconds())
	}
	return fields
}
