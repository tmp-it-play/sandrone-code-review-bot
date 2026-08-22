package summarizepullrequest

import (
	"log/slog"

	"github.com/it-play/sandrone-code-review-bot/internal/core/parsing"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

type Dependencies struct {
	Source    outbound.PullRequestSource
	Settings  outbound.SettingSource
	Masker    outbound.Masker
	Completer outbound.Completer
	Publisher outbound.ReviewPublisher
	Renderer  outbound.Renderer
	Reviews   outbound.ReviewRepository
	Clock     outbound.Clock
	Parser    parsing.ResultParser
	Logger    *slog.Logger
}
