package reviewworkflow

import "time"

type Unit struct {
	ID               uint64
	RunID            uint64
	Hash             string
	Ordinal          int
	Kind             string
	Status           UnitStatus
	AttemptCount     int
	Provider         string
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ErrorSummary     string
	StartedAt        *time.Time
	FinishedAt       *time.Time
	HeartbeatAt      *time.Time
	LeaseToken       string
	LeaseExpiresAt   *time.Time
}
