package selection

import "github.com/it-play/sandrone-code-review-bot/internal/core/review"

type SeverityFilter struct {
	Minimum review.Severity
}

func (f SeverityFilter) Apply(findings []review.Finding) []review.Finding {
	if f.Minimum.Rank() == 0 {
		return findings
	}
	kept := make([]review.Finding, 0, len(findings))
	for _, finding := range findings {
		if finding.Severity.AtLeast(f.Minimum) {
			kept = append(kept, finding)
		}
	}
	return kept
}
