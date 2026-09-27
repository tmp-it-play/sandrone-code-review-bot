package progresscheck

import (
	"log/slog"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

type Dependencies struct {
	Checks outbound.ProgressCheckPublisher
	Runs   outbound.ProgressCheckRunRepository
	Logger *slog.Logger
}
