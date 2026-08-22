package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type PullRequestSource interface {
	PullRequest(ctx context.Context, target pullrequest.Target) (pullrequest.PullRequest, error)
	ChangedFiles(ctx context.Context, target pullrequest.Target) ([]pullrequest.ChangedFile, error)
	ChangedFilesBetween(ctx context.Context, target pullrequest.Target, baseSHA string, headSHA string) ([]pullrequest.ChangedFile, error)
	FileContent(ctx context.Context, target pullrequest.Target, path string, ref string) (string, error)
}
