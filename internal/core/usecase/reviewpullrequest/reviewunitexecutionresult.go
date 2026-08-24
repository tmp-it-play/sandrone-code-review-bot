package reviewpullrequest

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewanalysis"
)

type reviewUnitExecutionResult struct {
	review            review.Result
	reduced           int
	response          llm.Response
	modelsUsed        map[string]struct{}
	multipleModels    bool
	reviewed          []pullrequest.ChangedFile
	failed            []pullrequest.ChangedFile
	schemaDropped     int
	evidenceReport    reviewanalysis.EvidenceReport
	reusedFindings    int
	reviewerProviders map[string]struct{}
	externalCalls     int
	budgetExhausted   bool
	errorMessage      string
}
