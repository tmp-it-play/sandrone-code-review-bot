package reviewenqueue

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type ProgressCheck interface {
	Show(ctx context.Context, target pullrequest.Target, marker string, message string) error
}
