package mapper

import (
	"encoding/json"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

func ToFindingModel(reviewID uint64, target pullrequest.Target, finding review.Finding) model.Finding {
	finding = finding.WithIdentity()
	occurrences, _ := json.Marshal(finding.Occurrences)
	return model.Finding{
		ReviewID:              reviewID,
		Owner:                 target.Owner,
		Repository:            target.Repository,
		Number:                target.Number,
		Fingerprint:           review.NewFingerprint(finding).String(),
		OccurrenceFingerprint: review.NewOccurrenceFingerprint(finding).String(),
		Path:                  finding.File,
		Line:                  finding.Line,
		EndLine:               finding.EndLine,
		Severity:              string(finding.Severity),
		Title:                 finding.Title,
		Body:                  finding.Body,
		Suggestion:            finding.Suggestion,
		Evidence:              finding.Evidence,
		RootCause:             finding.RootCause,
		OccurrencesJSON:       string(occurrences),
		Placement:             string(finding.Placement),
	}
}

func ToFinding(entry model.Finding) review.Finding {
	occurrences := []review.Occurrence{}
	_ = json.Unmarshal([]byte(entry.OccurrencesJSON), &occurrences)
	return review.Finding{
		File:         entry.Path,
		Line:         entry.Line,
		EndLine:      entry.EndLine,
		Severity:     review.Severity(entry.Severity),
		Title:        entry.Title,
		Body:         entry.Body,
		Suggestion:   entry.Suggestion,
		Evidence:     entry.Evidence,
		RootCause:    entry.RootCause,
		RootCauseID:  review.Fingerprint(entry.Fingerprint),
		OccurrenceID: review.Fingerprint(entry.OccurrenceFingerprint),
		Occurrences:  occurrences,
		Placement:    review.Placement(entry.Placement),
	}
}
