package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/progresscomment"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type ProgressCheckPublisher interface {
	CreateProgressCheck(ctx context.Context, target pullrequest.Target, externalID string, title string) (int64, error)
	UpdateProgressCheck(ctx context.Context, target pullrequest.Target, checkRunID int64, title string) error
	CompleteProgressCheck(ctx context.Context, target pullrequest.Target, checkRunID int64, conclusion progresscomment.CheckConclusion, title string) error
}
