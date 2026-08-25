package llm

type CompletionAttemptKind string

const (
	CompletionAttemptCooling           CompletionAttemptKind = "cooling"
	CompletionAttemptBusy              CompletionAttemptKind = "busy"
	CompletionAttemptPolicy            CompletionAttemptKind = "policy"
	CompletionAttemptPromptLimit       CompletionAttemptKind = "prompt_limit"
	CompletionAttemptOutputLimit       CompletionAttemptKind = "output_limit"
	CompletionAttemptProviderFailure   CompletionAttemptKind = "provider_failure"
	CompletionAttemptIncomplete        CompletionAttemptKind = "incomplete"
	CompletionAttemptInvalidJSON       CompletionAttemptKind = "invalid_json"
	CompletionAttemptInvalidSemantic   CompletionAttemptKind = "invalid_semantic"
	CompletionAttemptBudget            CompletionAttemptKind = "budget"
	CompletionAttemptBudgetUnavailable CompletionAttemptKind = "budget_unavailable"
)
