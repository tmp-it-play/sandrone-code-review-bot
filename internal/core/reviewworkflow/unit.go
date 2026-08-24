package reviewworkflow

import "time"

type Unit struct {
	ID               uint64
	RunID            uint64
	Hash             string
	InputHash        string
	Ordinal          int
	Kind             string
	Status           UnitStatus
	AttemptCount     int
	Provider         string
	Model            string
	MultipleModels   bool
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ToolExecutions   int
	ResultJSON       string
	Reused           bool
	ErrorSummary     string
	StartedAt        *time.Time
	FinishedAt       *time.Time
	HeartbeatAt      *time.Time
	LeaseToken       string
	LeaseExpiresAt   *time.Time
}
