package textrevision

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewsummary"
)

type Document struct {
	Source  review.Result
	Fields  []Field
	Compact bool
}

func NewDocument(source review.Result, compact bool) Document {
	document := Document{Source: source, Compact: compact}
	add := func(id, kind, text, context string, limit int) {
		if strings.TrimSpace(text) != "" {
			document.Fields = append(document.Fields, Field{ID: id, Kind: kind, Text: text, Context: context, Limit: max(limit, utf8.RuneCountInString(text))})
		}
	}
	overviewLimit := 2000
	if compact {
		overviewLimit = reviewsummary.MaxCharacters
	}
	add("overview", "summary", source.Summary.Overview, "", overviewLimit)
	for index, finding := range source.Findings {
		add(fmt.Sprintf("finding:%d", index), "finding.body", finding.Body, string(finding.Severity)+" | "+finding.Title+" | "+finding.RootCause, 800)
	}
	for index, note := range source.Summary.Files {
		add(fmt.Sprintf("file:%d", index), "summary.file", note.Note, "", 300)
	}
	return document
}

func (d Document) Apply(checkpoint Checkpoint) (review.Result, error) {
	if err := ValidateEdits(d.Fields, checkpoint.Edits, !checkpoint.Fallback); err != nil {
		return d.Source, err
	}
	edits := make(map[string]string, len(checkpoint.Edits))
	for _, edit := range checkpoint.Edits {
		if edit.ID == "overview" && d.Compact {
			if _, err := reviewsummary.CanonicalOverview(edit.Text); err != nil {
				return d.Source, errors.New("보정된 최종 요약 형식이 유효하지 않습니다")
			}
		}
		edits[edit.ID] = edit.Text
	}
	result := d.Source
	result.Summary.Files = slices.Clone(result.Summary.Files)
	result.Findings = slices.Clone(result.Findings)
	if text, exists := edits["overview"]; exists {
		result.Summary.Overview = text
	}
	for index := range result.Summary.Files {
		if text, exists := edits[fmt.Sprintf("file:%d", index)]; exists {
			result.Summary.Files[index].Note = text
		}
	}
	for index := range result.Findings {
		if text, exists := edits[fmt.Sprintf("finding:%d", index)]; exists {
			result.Findings[index].Body = text
		}
	}
	return result, nil
}
