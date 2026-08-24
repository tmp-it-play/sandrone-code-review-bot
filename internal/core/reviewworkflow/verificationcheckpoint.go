package reviewworkflow

import (
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
)

type VerificationCheckpoint struct {
	RunID                  uint64
	InputHash              string
	SupportedOccurrenceIDs []string
	Provider               string
	Model                  string
	MultipleModels         bool
	Usage                  llm.Usage
	ToolExecutions         int
	AttemptCount           int
	Completed              bool
	LastAttemptInputHash   string
	LastAttemptSucceeded   bool
	CompletedAt            time.Time
	FinishedAt             time.Time
}

func (c VerificationCheckpoint) MetadataResponse() llm.Response {
	provider := c.Provider
	model := c.Model
	label := ""
	if c.MultipleModels {
		provider = "multiple"
		model = "multiple"
		label = "복수 모델"
	}
	return llm.Response{
		Provider:       provider,
		Model:          model,
		ModelLabel:     label,
		Usage:          c.Usage,
		ToolExecutions: c.ToolExecutions,
	}
}
