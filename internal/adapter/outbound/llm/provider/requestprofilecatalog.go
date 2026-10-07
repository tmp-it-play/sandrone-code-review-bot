package provider

import (
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
)

func RequestProfileFor(providerName string, model string) RequestProfile {
	normalizedProvider := strings.ToLower(strings.TrimSpace(providerName))
	normalizedModel := strings.ToLower(strings.TrimSpace(model))
	baseModel, _, _ := strings.Cut(normalizedModel, ":")

	switch normalizedProvider + "/" + baseModel {
	case "gemini/gemini-3.7-flash":
		return RequestProfile{
			ReasoningEffort:         "medium",
			OutputTokenLimit:        65536,
			StructuredOutputPercent: 80,
			ForceJSONWithTools:      true,
		}
	case "groq/openai/gpt-oss-120b":
		capability := llm.Capability{JSONMode: true}
		return RequestProfile{
			Capability:              &capability,
			Temperature:             float64Value(1),
			TopP:                    float64Value(1),
			ReasoningEffort:         "low",
			ParallelToolCalls:       boolValue(false),
			OutputTokenLimit:        4096,
			StructuredOutputPercent: 50,
			UseMaxCompletionTokens:  true,
		}
	case "openrouter/nvidia/nemotron-3-super-120b-a12b":
		return RequestProfile{
			UseRequestTemperature:   true,
			NestedReasoningEffort:   "low",
			ExcludeReasoning:        true,
			OutputTokenLimit:        8192,
			StructuredOutputPercent: 50,
			ForceJSONWithTools:      true,
		}
	case "nvidia/google/gemma-4-31b-it":
		capability := llm.Capability{JSONMode: true}
		return RequestProfile{
			Capability:               &capability,
			Temperature:              float64Value(1),
			TopP:                     float64Value(0.95),
			ThinkingEnabled:          boolValue(false),
			OutputTokenLimit:         32768,
			StripLeadingEmptyThought: true,
		}
	case "mistral/mistral-small-2603":
		return RequestProfile{
			OutputTokenLimit:      8192,
			UseRequestTemperature: true,
			ForceJSONWithTools:    true,
		}
	case "cloudflare-glm/@cf/zai-org/glm-4.7-flash", "cloudflare-gemma/@cf/google/gemma-4-26b-a4b-it":
		return RequestProfile{
			ParallelToolCalls:      boolValue(false),
			OutputTokenLimit:       8192,
			UseMaxCompletionTokens: true,
			UseRequestTemperature:  true,
		}
	case "local/hf.co/ibm-granite/granite-4.1-3b-gguf":
		return RequestProfile{
			Temperature:      float64Value(0.2),
			TopP:             float64Value(0.95),
			OutputTokenLimit: 512,
		}
	default:
		if normalizedProvider == "local" {
			return RequestProfile{
				OutputTokenLimit:      512,
				UseRequestTemperature: true,
			}
		}
		return RequestProfile{UseRequestTemperature: true}
	}
}

func float64Value(value float64) *float64 {
	return &value
}

func boolValue(value bool) *bool {
	return &value
}
