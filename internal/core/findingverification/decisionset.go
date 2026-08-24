package findingverification

import (
	"sort"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type DecisionSet struct {
	entries map[string]Decision
}

func (s DecisionSet) Decision(occurrenceID review.Fingerprint) (Decision, bool) {
	decision, exists := s.entries[occurrenceID.String()]
	return decision, exists
}

func (s DecisionSet) Decisions() []Decision {
	decisions := make([]Decision, 0, len(s.entries))
	for _, decision := range s.entries {
		decisions = append(decisions, decision)
	}
	sort.Slice(decisions, func(left int, right int) bool {
		return decisions[left].OccurrenceID.String() < decisions[right].OccurrenceID.String()
	})
	return decisions
}

func (s DecisionSet) Supported(candidates []review.Finding) []review.Finding {
	supported := make([]review.Finding, 0, len(candidates))
	for _, candidate := range candidates {
		decision, exists := s.entries[occurrenceID(candidate).String()]
		if exists && decision.Status == StatusSupported {
			supported = append(supported, candidate)
		}
	}
	return supported
}
