package reviewpublication

import (
	"context"
	"errors"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

func (r *Reconciler) replaceTerminalProgressComment(ctx context.Context, target pullrequest.Target, marker string, reason string) error {
	body := strings.TrimSpace(reason) + "\n\n" + strings.TrimSpace(marker)
	commentIDs, err := r.deps.Publisher.FindComments(ctx, target, marker)
	if err != nil {
		return err
	}
	if len(commentIDs) > 0 {
		var updateErr error
		for _, commentID := range commentIDs {
			updateErr = errors.Join(updateErr, r.deps.Publisher.UpdateComment(ctx, target, commentID, body))
		}
		return updateErr
	}
	_, createErr := r.deps.Publisher.CreateComment(ctx, target, body)
	if createErr == nil {
		return nil
	}
	commentID, exists, reconcileErr := r.deps.Publisher.FindComment(ctx, target, marker)
	if !exists {
		return errors.Join(createErr, reconcileErr)
	}
	return errors.Join(createErr, reconcileErr, r.deps.Publisher.UpdateComment(ctx, target, commentID, body))
}
