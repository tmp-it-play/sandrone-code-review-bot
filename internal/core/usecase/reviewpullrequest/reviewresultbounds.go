package reviewpullrequest

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

const maxPublishedFindings = 25

func boundedBatchResult(files []pullrequest.ChangedFile, result review.Result, includeFileNotes bool) review.Result {
	allowed := make(map[string]struct{}, len(files))
	for _, file := range files {
		allowed[file.Path] = struct{}{}
	}
	result.Summary.Overview = boundedRunes(result.Summary.Overview, 2000)
	notes := make([]review.FileNote, 0, len(result.Summary.Files))
	if includeFileNotes {
		seen := map[string]struct{}{}
		for _, note := range result.Summary.Files {
			if _, exists := allowed[note.Path]; !exists {
				continue
			}
			if _, exists := seen[note.Path]; exists {
				continue
			}
			seen[note.Path] = struct{}{}
			note.Note = boundedRunes(note.Note, 300)
			notes = append(notes, note)
		}
	}
	result.Summary.Files = notes
	for index := range result.Findings {
		result.Findings[index].Title = boundedRunes(result.Findings[index].Title, 200)
		result.Findings[index].Body = boundedRunes(result.Findings[index].Body, 800)
		result.Findings[index].RootCause = boundedRunes(result.Findings[index].RootCause, 500)
		if utf8.RuneCountInString(result.Findings[index].Suggestion) > 500 {
			result.Findings[index].Suggestion = ""
		}
		if utf8.RuneCountInString(result.Findings[index].Evidence) > 1600 {
			result.Findings[index].Evidence = ""
		}
	}
	return result
}

func publishedFindingBudget(findings []review.Finding) ([]review.Finding, int) {
	groups := make(map[string][]review.Finding, len(findings))
	for _, finding := range findings {
		identified := finding.WithIdentity()
		key := identified.RootCauseID.String()
		groups[key] = append(groups[key], identified)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
		sort.SliceStable(groups[key], func(left int, right int) bool {
			return findingPrecedes(groups[key][left], groups[key][right])
		})
	}
	sort.SliceStable(keys, func(left int, right int) bool {
		leftFinding := groups[keys[left]][0]
		rightFinding := groups[keys[right]][0]
		if findingPrecedes(leftFinding, rightFinding) {
			return true
		}
		if findingPrecedes(rightFinding, leftFinding) {
			return false
		}
		return keys[left] < keys[right]
	})
	selected := make([]review.Finding, 0, min(len(findings), maxPublishedFindings))
	for occurrenceIndex := 0; len(selected) < maxPublishedFindings; occurrenceIndex++ {
		added := false
		for _, key := range keys {
			if occurrenceIndex >= len(groups[key]) {
				continue
			}
			selected = append(selected, groups[key][occurrenceIndex])
			added = true
			if len(selected) == maxPublishedFindings {
				break
			}
		}
		if !added {
			break
		}
	}
	return selected, len(findings) - len(selected)
}

func findingPrecedes(left review.Finding, right review.Finding) bool {
	if left.Severity.Rank() != right.Severity.Rank() {
		return left.Severity.Rank() > right.Severity.Rank()
	}
	if left.File != right.File {
		return left.File < right.File
	}
	if left.Line != right.Line {
		return left.Line < right.Line
	}
	if left.EndLine != right.EndLine {
		return left.EndLine < right.EndLine
	}
	return review.NewOccurrenceFingerprint(left).String() < review.NewOccurrenceFingerprint(right).String()
}

func boundedRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	characters := []rune(value)
	if limit <= 0 || len(characters) <= limit {
		return value
	}
	return strings.TrimSpace(string(characters[:limit])) + "…"
}
