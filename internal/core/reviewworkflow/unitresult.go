package reviewworkflow

import (
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
)

type UnitResult struct {
	Status     UnitStatus
	Provider   string
	Model      string
	Usage      llm.Usage
	Error      string
	FinishedAt time.Time
}
