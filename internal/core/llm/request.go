package llm

import "time"

type Request struct {
	Messages              []Message
	Temperature           float64
	MaxOutputTokens       int
	RequiredOutputTokens  int
	Tools                 []Tool
	ForceJSON             bool
	Providers             []string
	ExcludedProviders     []string
	TaskRole              TaskRole
	DataClassification    DataClassification
	ExternalCallBudget    *ExternalCallBudget
	RequireCompletePrompt bool
	FailFastOnIncomplete  bool
	ResponseValidation    ResponseValidation
	Deadline              time.Time
}

func (r Request) WithMessages(messages []Message) Request {
	r.Messages = messages
	return r
}
