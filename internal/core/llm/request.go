package llm

import "time"

type Request struct {
	Messages              []Message
	FitMessages           func(int) ([]Message, bool)         `json:"-"`
	PlanWindow            func(int, int) (PromptWindow, bool) `json:"-"`
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
	ResponseSchema        *ResponseSchema
	Deadline              time.Time
}

func (r Request) WithMessages(messages []Message) Request {
	r.Messages = messages
	r.FitMessages = nil
	r.PlanWindow = nil
	return r
}
