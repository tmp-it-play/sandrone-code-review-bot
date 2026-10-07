package reviewpullrequest

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/batching"
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

type reviewUnitExecution struct {
	target            pullrequest.Target
	runID             uint64
	runConfigHash     string
	runLease          string
	config            setting.RepoConfig
	planner           reviewBatchPlanner
	plan              batching.Plan
	files             []pullrequest.ChangedFile
	promptConfig      setting.RepoConfig
	units             []reviewworkflow.Unit
	coverage          []reviewworkflow.CoverageItem
	request           llm.Request
	paths             *promptPathMap
	includeFileNotes  bool
	maxCalls          int
	reviewerCallLimit int
	externalCalls     int
}
