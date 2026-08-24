package reviewpullrequest

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

type reviewResultFinalization struct {
	target      pullrequest.Target
	runID       uint64
	runLease    string
	config      setting.RepoConfig
	execution   reviewUnitExecutionResult
	request     llm.Request
	paths       *promptPathMap
	trigger     review.Trigger
	incremental bool
	runStatus   reviewworkflow.RunStatus
}
