package replythread

import (
	"log/slog"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

type Dependencies struct {
	Source       outbound.PullRequestSource
	Settings     outbound.SettingSource
	Masker       outbound.Masker
	Completer    outbound.Completer
	Threads      outbound.ThreadPublisher
	Renderer     outbound.Renderer
	Publications outbound.ReplyPublicationRepository
	Clock        outbound.Clock
	Logger       *slog.Logger
	Retention    time.Duration
}
