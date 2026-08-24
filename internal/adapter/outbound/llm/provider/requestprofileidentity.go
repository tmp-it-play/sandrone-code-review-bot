package provider

import "github.com/it-play/sandrone-code-review-bot/internal/core/llm"

func (p RequestProfile) Identity(request llm.Request) llm.ProviderRequestPolicyIdentity {
	temperature := cloneFloat(p.Temperature)
	if p.UseRequestTemperature && request.Temperature != 0 {
		value := request.Temperature
		temperature = &value
	}
	maxOutputTokens := request.MaxOutputTokens
	if p.OutputTokenLimit > 0 && maxOutputTokens > p.OutputTokenLimit {
		maxOutputTokens = p.OutputTokenLimit
	}
	identity := llm.ProviderRequestPolicyIdentity{
		Temperature:           temperature,
		TopP:                  cloneFloat(p.TopP),
		ReasoningEffort:       p.ReasoningEffort,
		NestedReasoningEffort: p.NestedReasoningEffort,
		ThinkingEnabled:       cloneBool(p.ThinkingEnabled),
		ParallelToolCalls:     cloneBool(p.ParallelToolCalls),
		MaxOutputTokens:       maxOutputTokens,
		MaxCompletionTokens:   p.UseMaxCompletionTokens,
	}
	if p.NestedReasoningEffort != "" {
		identity.ExcludeReasoning = p.ExcludeReasoning
	}
	return identity
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
