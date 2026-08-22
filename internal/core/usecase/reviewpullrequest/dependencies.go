package reviewpullrequest

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
	Tools     outbound.ToolExecutorFactory
	Publisher outbound.ReviewPublisher
	Renderer  outbound.Renderer
	Reviews   outbound.ReviewRepository
	Findings  outbound.FindingRepository
	State     outbound.PullRequestStateRepository
	Clock     outbound.Clock
	Parser    parsing.ResultParser
	Logger    *slog.Logger
}
