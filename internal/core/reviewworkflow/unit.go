package reviewworkflow

import "time"

type Unit struct {
	ID               uint64
	RunID            uint64
	Hash             string
	ParentHash       string
	InputHash        string
	Ordinal          int
	Depth            int
	OrderKey         string
	Kind             string
	SpecJSON         string
	SplitHash        string
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
	Retryable        bool
	RetryAt          *time.Time
	ErrorSummary     string
	StartedAt        *time.Time
	FinishedAt       *time.Time
	HeartbeatAt      *time.Time
	LeaseToken       string
	LeaseExpiresAt   *time.Time
}
