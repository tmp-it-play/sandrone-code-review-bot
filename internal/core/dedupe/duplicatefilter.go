package dedupe

import "github.com/it-play/sandrone-code-review-bot/internal/core/review"

type DuplicateFilter struct {
	Known map[string]struct{}
}

func (f DuplicateFilter) Apply(findings []review.Finding) ([]review.Finding, int) {
	seen := map[string]struct{}{}
	kept := make([]review.Finding, 0, len(findings))
	skipped := 0
	for _, finding := range findings {
		key := review.NewFingerprint(finding).String()
		if _, ok := f.Known[key]; ok {
			skipped++
			continue
		}
		if _, ok := seen[key]; ok {
			skipped++
			continue
		}
		seen[key] = struct{}{}
		kept = append(kept, finding)
	}
	return kept, skipped
}
