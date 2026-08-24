package publication

import (
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
)

type CompletionCheckpoint struct {
	InputHash        string
	CanonicalContent string
	Provider         string
	Model            string
	ModelLabel       string
	FinishReason     string
	MultipleModels   bool
	Usage            llm.Usage
	ToolExecutions   int
	CompletedAt      time.Time
}

func NewCompletionCheckpoint(inputHash string, canonicalContent string, response llm.Response, completedAt time.Time) CompletionCheckpoint {
	return CompletionCheckpoint{
		InputHash:        inputHash,
		CanonicalContent: canonicalContent,
		Provider:         response.Provider,
		Model:            response.Model,
		ModelLabel:       response.ModelLabel,
		FinishReason:     response.FinishReason,
		MultipleModels:   response.ModelLabel == "복수 모델" || response.Provider == "multiple" || response.Model == "multiple",
		Usage:            response.Usage,
		ToolExecutions:   response.ToolExecutions,
		CompletedAt:      completedAt,
	}
}

func (c CompletionCheckpoint) MetadataResponse() llm.Response {
	provider := c.Provider
	model := c.Model
	label := c.ModelLabel
	if c.MultipleModels {
		provider = "multiple"
		model = "multiple"
		label = "복수 모델"
	}
	return llm.Response{
		Provider:       provider,
		Model:          model,
		ModelLabel:     label,
		FinishReason:   c.FinishReason,
		Usage:          c.Usage,
		ToolExecutions: c.ToolExecutions,
	}
}
