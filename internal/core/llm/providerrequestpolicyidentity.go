package llm

type ProviderRequestPolicyIdentity struct {
	Temperature           *float64
	TopP                  *float64
	ReasoningEffort       string
	NestedReasoningEffort string
	ExcludeReasoning      bool
	ThinkingEnabled       *bool
	ParallelToolCalls     *bool
	MaxOutputTokens       int
	UsableOutputTokens    int
	MaxCompletionTokens   bool
	ForceJSONWithTools    bool
	StripEmptyThought     bool
}
