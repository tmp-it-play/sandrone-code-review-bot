package llm

type PolicyHashInputs struct {
	Version               string
	TaskRole              TaskRole
	DataClassification    DataClassification
	RequestedProviders    []string
	ExcludedProviders     []string
	EligibleProviders     []ProviderPolicyIdentity
	MaxExternalCalls      int
	MaxToolRounds         int
	MaxTransientRetries   int
	RequiredOutputTokens  int
	ForceJSON             bool
	RequireCompletePrompt bool
	FailFastOnIncomplete  bool
	ResponseValidation    ResponseValidation
}
