package reviewworkflow

import (
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type UnitResult struct {
	Status         UnitStatus
	InputHash      string
	Provider       string
	Model          string
	MultipleModels bool
	Usage          llm.Usage
	Review         review.Result
	ToolExecutions int
	Reused         bool
	Error          string
	FinishedAt     time.Time
}
