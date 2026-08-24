package reviewpullrequest

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type reviewResultFinalizationResult struct {
	draft                   review.Result
	view                    review.SummaryView
	attribution             review.Attribution
	style                   review.Style
	response                llm.Response
	failureResponse         llm.Response
	runStatus               reviewworkflow.RunStatus
	verificationUnavailable bool
	errorMessage            string
}
