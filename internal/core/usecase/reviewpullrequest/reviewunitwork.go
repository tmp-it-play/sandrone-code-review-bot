package reviewpullrequest

import (
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type reviewUnitWork struct {
	unit  reviewworkflow.Unit
	files []pullrequest.ChangedFile
}

func (w reviewUnitWork) reservesCall() bool {
	switch w.unit.Status {
	case reviewworkflow.UnitStatusPending, reviewworkflow.UnitStatusRunning:
		return true
	case reviewworkflow.UnitStatusFailed:
		return w.unit.Retryable
	default:
		return false
	}
}

func (w reviewUnitWork) terminalFailure(inputHash string) bool {
	return w.unit.Status == reviewworkflow.UnitStatusFailed && !w.unit.Retryable && w.unit.InputHash == inputHash
}

func (w reviewUnitWork) terminalDeferred(inputHash string) bool {
	return w.unit.Status == reviewworkflow.UnitStatusDeferred && w.unit.InputHash == inputHash
}

func (w reviewUnitWork) deferredRetry(inputHash string, at time.Time) bool {
	return w.unit.Status == reviewworkflow.UnitStatusFailed && w.unit.Retryable && w.unit.InputHash == inputHash && w.unit.RetryAt != nil && w.unit.RetryAt.After(at)
}
