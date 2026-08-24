package reviewpullrequest

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type findingVerificationResult struct {
	findings    []review.Finding
	response    llm.Response
	rejected    int
	used        bool
	unavailable bool
}
