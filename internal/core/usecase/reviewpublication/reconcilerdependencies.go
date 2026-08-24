package reviewpublication

import (
	"log/slog"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

type ReconcilerDependencies struct {
	Workflows outbound.ReviewPublicationRecoveryRepository
	Source    outbound.PullRequestSource
	Publisher outbound.ReviewPublisher
	Reviews   outbound.ReviewRepository
	Clock     outbound.Clock
	Logger    *slog.Logger
}
