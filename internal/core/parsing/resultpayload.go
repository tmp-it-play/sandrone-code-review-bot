package parsing

import (
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type resultPayload struct {
	Summary struct {
		Overview string `json:"overview"`
		Files    []struct {
			Path string `json:"path"`
			Note string `json:"note"`
		} `json:"files"`
	} `json:"summary"`
	Findings []struct {
		File       string `json:"file"`
		Line       int    `json:"line"`
		EndLine    int    `json:"endLine"`
		Severity   string `json:"severity"`
		Title      string `json:"title"`
		Body       string `json:"body"`
		Suggestion string `json:"suggestion"`
		Evidence   string `json:"evidence"`
		RootCause  string `json:"rootCause"`
	} `json:"findings"`
}

func (p resultPayload) toDomain() (review.Result, int) {
	result := review.Result{}
	dropped := 0
	result.Summary.Overview = strings.TrimSpace(p.Summary.Overview)
	for _, file := range p.Summary.Files {
		path := strings.TrimSpace(file.Path)
		if path == "" {
			continue
		}
		result.Summary.Files = append(result.Summary.Files, review.FileNote{Path: path, Note: strings.TrimSpace(file.Note)})
	}
	for _, entry := range p.Findings {
		severity, ok := review.ParseSeverity(entry.Severity)
		if !ok {
			dropped++
			continue
		}
		finding := review.Finding{
			File:       strings.TrimSpace(entry.File),
			Line:       entry.Line,
			EndLine:    entry.EndLine,
			Severity:   severity,
			Title:      strings.TrimSpace(entry.Title),
			Body:       strings.TrimSpace(entry.Body),
			Suggestion: entry.Suggestion,
			Evidence:   entry.Evidence,
			RootCause:  strings.TrimSpace(entry.RootCause),
		}
		if !finding.IsValid() {
			dropped++
			continue
		}
		result.Findings = append(result.Findings, finding)
	}
	return result, dropped
}
