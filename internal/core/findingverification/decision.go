package findingverification

import "github.com/it-play/sandrone-code-review-bot/internal/core/review"

type Decision struct {
	OccurrenceID review.Fingerprint
	Status       Status
	Reason       string
}
