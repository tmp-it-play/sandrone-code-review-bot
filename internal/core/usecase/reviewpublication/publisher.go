package reviewpublication

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type Publisher interface {
	PublicationExists(ctx context.Context, target pullrequest.Target, marker string) (bool, error)
	SubmitReview(ctx context.Context, target pullrequest.Target, marker string, body string, comments []review.InlineComment) (int64, error)
	CreateComment(ctx context.Context, target pullrequest.Target, body string) (int64, error)
}
