package handlecommand

import (
	"log/slog"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

type Dependencies struct {
	Permissions outbound.PermissionChecker
	Reactions   outbound.ReactionPublisher
	Threads     outbound.ThreadPublisher
	Publisher   outbound.ReviewPublisher
	Renderer    outbound.Renderer
	Queue       outbound.Queue
	Commands    outbound.CommandRepository
	Clock       outbound.Clock
	Logger      *slog.Logger
}
