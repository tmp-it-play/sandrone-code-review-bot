package usage

import "time"

type Event struct {
	Provider                   string
	Model                      string
	Role                       string
	Outcome                    string
	Status                     int
	ProviderErrorCode          string
	RequestElapsedMilliseconds int64
	PromptTokens               int
	CompletionTokens           int
	TotalTokens                int
	OccurredAt                 time.Time
}
