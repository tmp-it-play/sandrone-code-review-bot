package reviewpullrequest

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func (f *reviewResultFinalizer) refineText(ctx context.Context, input reviewResultFinalization, draft review.Result) (review.Result, llm.Response, error) {
	plan, err := f.deps.refiner.Plan(draft, input.config, input.request, true)
	if err != nil {
		return draft, llm.Response{}, err
	}
	checkpoint, found, err := f.deps.revisions.ReviewTextRevision(ctx, input.runID, input.runLease)
	if err != nil {
		return draft, llm.Response{}, err
	}
	if found && checkpoint.InputHash != plan.InputHash {
		return draft, checkpoint.Response, reviewworkflow.ErrPlanChanged
	}
	if !found {
		checkpoint = f.deps.refiner.Refine(ctx, plan)
		checkpointContext, cancel := durableCheckpointContext(ctx)
		err = f.deps.revisions.SaveReviewTextRevision(checkpointContext, input.runID, input.runLease, checkpoint)
		cancel()
		if err != nil {
			return draft, checkpoint.Response, err
		}
	}
	f.deps.logger.Info("최종 문체 보정", "run", input.runID, "reused", found, "fallback", checkpoint.Fallback, "edited", len(checkpoint.Edits))
	result, err := plan.Document.Apply(checkpoint)
	return result, checkpoint.Response, err
}
