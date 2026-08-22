package replythread

import (
	"log/slog"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

type Dependencies struct {
	Source    outbound.PullRequestSource
	Settings  outbound.SettingSource
	Masker    outbound.Masker
	Completer outbound.Completer
	Threads   outbound.ThreadPublisher
	Publisher outbound.ReviewPublisher
	Renderer  outbound.Renderer
	Clock     outbound.Clock
	Logger    *slog.Logger
}
