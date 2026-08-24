package reviewpullrequest

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type findingFingerprintRepository interface {
	Fingerprints(ctx context.Context, target pullrequest.Target) (map[string]struct{}, error)
}
