package reviewpullrequest

import (
	"log/slog"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/refinepublication"
)

type reviewResultFinalizerDependencies struct {
	findings  findingFingerprintRepository
	verifier  *findingVerifier
	summaries *reviewSummaryReducer
	revisions outbound.ReviewSummaryRepository
	refiner   *refinepublication.Refiner
	logger    *slog.Logger
}
