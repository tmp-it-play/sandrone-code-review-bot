package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/access"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type PermissionChecker interface {
	Permission(ctx context.Context, target pullrequest.Target, username string) (access.Permission, error)
}
