package reviewpublication

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type Publisher interface {
	VerifyTarget(ctx context.Context, target pullrequest.Target) error
	PublicationExists(ctx context.Context, target pullrequest.Target, marker string) (bool, error)
	SubmitReview(ctx context.Context, target pullrequest.Target, marker string, body string, comments []review.InlineComment) (int64, error)
	CreateComment(ctx context.Context, target pullrequest.Target, body string) (int64, error)
	UpdateComment(ctx context.Context, target pullrequest.Target, commentID int64, body string) error
	FindComment(ctx context.Context, target pullrequest.Target, marker string) (int64, bool, error)
	FindComments(ctx context.Context, target pullrequest.Target, marker string) ([]int64, error)
}
