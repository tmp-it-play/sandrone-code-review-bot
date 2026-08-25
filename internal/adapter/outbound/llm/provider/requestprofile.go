package provider

import "github.com/it-play/sandrone-code-review-bot/internal/core/llm"

type RequestProfile struct {
	Capability               *llm.Capability
	Temperature              *float64
	TopP                     *float64
	ReasoningEffort          string
	NestedReasoningEffort    string
	ExcludeReasoning         bool
	ThinkingEnabled          *bool
	ParallelToolCalls        *bool
	OutputTokenLimit         int
	StructuredOutputPercent  int
	UseMaxCompletionTokens   bool
	UseRequestTemperature    bool
	ForceJSONWithTools       bool
	StripLeadingEmptyThought bool
}
