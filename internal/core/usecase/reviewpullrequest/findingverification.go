package reviewpullrequest

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type findingVerification struct {
	runID             uint64
	runLease          string
	findings          []review.Finding
	files             []pullrequest.ChangedFile
	request           llm.Request
	reviewerProviders map[string]struct{}
	paths             *promptPathMap
}
