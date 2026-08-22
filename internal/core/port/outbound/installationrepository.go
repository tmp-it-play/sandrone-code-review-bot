package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/installation"
)

type InstallationRepository interface {
	Upsert(ctx context.Context, entry installation.Installation) error
	UpsertRepository(ctx context.Context, entry installation.Repository) error
	Repositories(ctx context.Context) ([]installation.Repository, error)
}
