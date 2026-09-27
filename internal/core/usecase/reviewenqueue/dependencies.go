package reviewenqueue

import (
	"log/slog"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

type Dependencies struct {
	Queue     AdmissionQueue
	Source    outbound.PullRequestSource
	Settings  outbound.SettingSource
	Runs      outbound.ReviewRunRepository
	Publisher outbound.ReviewPublisher
	Renderer  outbound.Renderer
	Checks    ProgressCheck
	Clock     outbound.Clock
	Logger    *slog.Logger
}
