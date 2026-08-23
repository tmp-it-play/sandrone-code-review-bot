package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type ReactionPublisher interface {
	AddReaction(ctx context.Context, target pullrequest.Target, commentID int64, inThread bool, reaction string) error
	AddPullRequestReaction(ctx context.Context, target pullrequest.Target, reaction string) error
}
