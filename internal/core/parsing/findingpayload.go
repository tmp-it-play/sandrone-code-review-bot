package parsing

import (
	"strings"
	"unicode/utf8"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type findingPayload struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	EndLine    int    `json:"endLine"`
	Severity   string `json:"severity"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	Suggestion string `json:"suggestion"`
	Evidence   string `json:"evidence"`
	RootCause  string `json:"rootCause"`
}

func (p findingPayload) toDomain() (review.Finding, bool) {
	if utf8.RuneCountInString(p.Evidence) > 1600 {
		return review.Finding{}, false
	}
	severity, ok := review.ParseSeverity(p.Severity)
	if !ok {
		return review.Finding{}, false
	}
	finding := review.Finding{
		File:       strings.TrimSpace(p.File),
		Line:       p.Line,
		EndLine:    p.EndLine,
		Severity:   severity,
		Title:      strings.TrimSpace(p.Title),
		Body:       strings.TrimSpace(p.Body),
		Suggestion: p.Suggestion,
		Evidence:   p.Evidence,
		RootCause:  strings.TrimSpace(p.RootCause),
	}
	return finding, finding.IsValid()
}
