package mapper

import (
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

func ToFindingModel(reviewID uint64, target pullrequest.Target, finding review.Finding) model.Finding {
	return model.Finding{
		ReviewID:    reviewID,
		Owner:       target.Owner,
		Repository:  target.Repository,
		Number:      target.Number,
		Fingerprint: review.NewFingerprint(finding).String(),
		Path:        finding.File,
		Line:        finding.Line,
		Severity:    string(finding.Severity),
		Title:       finding.Title,
		Body:        finding.Body,
		Suggestion:  finding.Suggestion,
		Placement:   string(finding.Placement),
	}
}

func ToFinding(entry model.Finding) review.Finding {
	return review.Finding{
		File:       entry.Path,
		Line:       entry.Line,
		Severity:   review.Severity(entry.Severity),
		Title:      entry.Title,
		Body:       entry.Body,
		Suggestion: entry.Suggestion,
		Placement:  review.Placement(entry.Placement),
	}
}
