package findingverification

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

func occurrenceID(finding review.Finding) review.Fingerprint {
	if finding.OccurrenceID.String() != "" {
		return finding.OccurrenceID
	}
	return review.NewOccurrenceFingerprint(finding)
}
