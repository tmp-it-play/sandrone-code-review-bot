package summarizepullrequest

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/refinepublication"
)

func (u *UseCase) refineText(ctx context.Context, target pullrequest.Target, operationKey string, lease string, summary review.Summary, config setting.RepoConfig, request llm.Request) (review.Summary, llm.Response, error) {
	refiner := refinepublication.New(u.deps.Completer, u.deps.Masker, u.deps.Logger)
	plan, err := refiner.Plan(review.Result{Summary: summary}, config, request, false)
	if err != nil {
		return summary, llm.Response{}, err
	}
	checkpoint, found, err := u.deps.Publications.SummaryTextRevision(ctx, target, operationKey, lease, plan.InputHash)
	if err != nil {
		return summary, llm.Response{}, err
	}
	if !found {
		checkpoint = refiner.Refine(ctx, plan)
		checkpointContext, cancel := completionCheckpointContext(ctx)
		err = u.deps.Publications.SaveSummaryTextRevision(checkpointContext, target, operationKey, lease, checkpoint)
		cancel()
		if err != nil {
			return summary, checkpoint.Response, err
		}
	}
	u.deps.Logger.Info("요약 문체 보정", "target", target.Reference(), "reused", found, "fallback", checkpoint.Fallback, "edited", len(checkpoint.Edits))
	result, err := plan.Document.Apply(checkpoint)
	return result.Summary, checkpoint.Response, err
}
