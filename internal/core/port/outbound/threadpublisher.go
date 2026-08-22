package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/thread"
)

type ThreadPublisher interface {
	Thread(ctx context.Context, target pullrequest.Target, commentID int64) (thread.Thread, error)
	Reply(ctx context.Context, target pullrequest.Target, commentID int64, body string) error
}
