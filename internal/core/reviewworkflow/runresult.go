package reviewworkflow

import (
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type RunResult struct {
	Status                  RunStatus
	ReviewOutcome           review.Outcome
	Error                   string
	TerminalAt              time.Time
	ExpiresAt               time.Time
	AdvanceWatermark        bool
	LeaseToken              string
	PublicationInvalidation *PublicationInvalidation
}
