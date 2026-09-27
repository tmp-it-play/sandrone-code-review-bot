package refreshprogresscomment

import (
	"log/slog"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

type Dependencies struct {
	Refreshes outbound.ProgressCommentRefreshRepository
	Publisher outbound.ReviewPublisher
	Renderer  outbound.Renderer
	Checks    ProgressCheck
	Clock     outbound.Clock
	Logger    *slog.Logger
}
