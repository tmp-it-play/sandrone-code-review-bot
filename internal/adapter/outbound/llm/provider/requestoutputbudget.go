package provider

import "github.com/it-play/sandrone-code-review-bot/internal/core/llm"

func (p RequestProfile) outputTokens(request llm.Request) int {
	tokens := request.MaxOutputTokens
	required := request.RequiredOutputTokens
	if p.StructuredOutputPercent > 0 && p.StructuredOutputPercent < 100 {
		required = (required*100 + p.StructuredOutputPercent - 1) / p.StructuredOutputPercent
	}
	if tokens < required {
		tokens = required
	}
	if p.OutputTokenLimit > 0 && tokens > p.OutputTokenLimit {
		tokens = p.OutputTokenLimit
	}
	return tokens
}
