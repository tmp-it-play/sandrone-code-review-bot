package reviewpullrequest

import (
	"log/slog"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

type findingVerifierDependencies struct {
	verification outbound.ReviewVerificationRepository
	completer    reviewVerificationCompleter
	masker       outbound.Masker
	clock        outbound.Clock
	logger       *slog.Logger
}
