package reviewpublication

import (
	"context"
	"errors"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/progresscomment"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

func (r *Reconciler) replaceTerminalProgressComment(ctx context.Context, target pullrequest.Target, marker string, reason string) error {
	body := strings.TrimSpace(reason) + "\n\n" + strings.TrimSpace(marker)
	comments, err := r.deps.Publisher.FindComments(ctx, target, marker)
	if err != nil {
		return err
	}
	var updateErr error
	for _, comment := range comments {
		if comment.Body == body {
			continue
		}
		updateErr = errors.Join(updateErr, r.deps.Publisher.UpdateComment(ctx, target, comment.ID, body))
	}
	r.deps.Checks.Complete(context.WithoutCancel(ctx), target, []string{marker}, progresscomment.CheckConclusionNeutral, reason)
	return updateErr
}
