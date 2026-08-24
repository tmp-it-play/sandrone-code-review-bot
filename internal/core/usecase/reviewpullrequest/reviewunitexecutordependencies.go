package reviewpullrequest

import (
	"log/slog"

	"github.com/it-play/sandrone-code-review-bot/internal/core/parsing"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

type reviewUnitExecutorDependencies struct {
	execution outbound.ReviewExecutionRepository
	completer outbound.Completer
	tools     outbound.ToolExecutorFactory
	masker    outbound.Masker
	clock     outbound.Clock
	parser    parsing.ResultParser
	logger    *slog.Logger
}
