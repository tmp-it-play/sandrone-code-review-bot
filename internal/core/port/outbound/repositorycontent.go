package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type RepositoryContent interface {
	File(ctx context.Context, target pullrequest.Target, path string, ref string) (string, error)
	Paths(ctx context.Context, target pullrequest.Target, ref string) ([]string, error)
}
