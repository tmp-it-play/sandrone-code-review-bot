package reviewpullrequest

import (
	"log/slog"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

type findingVerifierDependencies struct {
	workflows reviewVerificationRepository
	completer reviewVerificationCompleter
	masker    outbound.Masker
	clock     outbound.Clock
	logger    *slog.Logger
}
