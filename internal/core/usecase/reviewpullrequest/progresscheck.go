package reviewpullrequest

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/progresscomment"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type ProgressCheck interface {
	Complete(ctx context.Context, target pullrequest.Target, markers []string, conclusion progresscomment.CheckConclusion, message string)
}
