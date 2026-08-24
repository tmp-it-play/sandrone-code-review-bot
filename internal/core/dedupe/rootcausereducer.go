package dedupe

import (
	"sort"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type RootCauseReducer struct{}

func (r RootCauseReducer) Apply(findings []review.Finding) ([]review.Finding, int) {
	groups := map[string][]review.Finding{}
	for _, finding := range findings {
		identified := finding.WithIdentity()
		key := identified.RootCauseID.String()
		groups[key] = append(groups[key], identified)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	reduced := make([]review.Finding, 0, len(keys))
	for _, key := range keys {
		group := groups[key]
		selected := group[0]
		occurrences := map[string]review.Occurrence{}
		for _, finding := range group {
			if preferFinding(finding, selected) {
				selected = finding
			}
			for _, occurrence := range finding.Occurrences {
				if occurrence.EndLine == occurrence.Line {
					occurrence.EndLine = 0
				}
				current, exists := occurrences[occurrence.ID.String()]
				if !exists || preferOccurrence(occurrence, current) {
					occurrences[occurrence.ID.String()] = occurrence
				}
			}
		}
		selected.Occurrences = orderedOccurrences(occurrences)
		if selected.EndLine == selected.Line {
			selected.EndLine = 0
		}
		reduced = append(reduced, selected)
	}
	sort.SliceStable(reduced, func(left, right int) bool {
		if reduced[left].Severity.Rank() != reduced[right].Severity.Rank() {
			return reduced[left].Severity.Rank() > reduced[right].Severity.Rank()
		}
		if reduced[left].File != reduced[right].File {
			return reduced[left].File < reduced[right].File
		}
		if reduced[left].Line != reduced[right].Line {
			return reduced[left].Line < reduced[right].Line
		}
		return reduced[left].RootCauseID.String() < reduced[right].RootCauseID.String()
	})
	return reduced, len(findings) - len(reduced)
}

func preferFinding(candidate review.Finding, current review.Finding) bool {
	if candidate.Severity.Rank() != current.Severity.Rank() {
		return candidate.Severity.Rank() > current.Severity.Rank()
	}
	if (strings.TrimSpace(candidate.Suggestion) != "") != (strings.TrimSpace(current.Suggestion) != "") {
		return strings.TrimSpace(candidate.Suggestion) != ""
	}
	if len(candidate.Body) != len(current.Body) {
		return len(candidate.Body) > len(current.Body)
	}
	if candidate.File != current.File {
		return candidate.File < current.File
	}
	if candidate.Line != current.Line {
		return candidate.Line < current.Line
	}
	if candidate.EndLine != current.EndLine {
		return candidate.EndLine < current.EndLine
	}
	if candidate.Title != current.Title {
		return candidate.Title < current.Title
	}
	if candidate.Body != current.Body {
		return candidate.Body < current.Body
	}
	if candidate.Suggestion != current.Suggestion {
		return candidate.Suggestion < current.Suggestion
	}
	if candidate.Evidence != current.Evidence {
		return candidate.Evidence < current.Evidence
	}
	if candidate.RootCause != current.RootCause {
		return candidate.RootCause < current.RootCause
	}
	return candidate.OccurrenceID.String() < current.OccurrenceID.String()
}

func preferOccurrence(candidate review.Occurrence, current review.Occurrence) bool {
	if candidate.File != current.File {
		return candidate.File < current.File
	}
	if candidate.Line != current.Line {
		return candidate.Line < current.Line
	}
	if candidate.EndLine != current.EndLine {
		return candidate.EndLine < current.EndLine
	}
	if candidate.Evidence != current.Evidence {
		return candidate.Evidence < current.Evidence
	}
	return candidate.ID.String() < current.ID.String()
}

func orderedOccurrences(occurrences map[string]review.Occurrence) []review.Occurrence {
	ordered := make([]review.Occurrence, 0, len(occurrences))
	for _, occurrence := range occurrences {
		ordered = append(ordered, occurrence)
	}
	sort.Slice(ordered, func(left, right int) bool {
		if ordered[left].File != ordered[right].File {
			return ordered[left].File < ordered[right].File
		}
		if ordered[left].Line != ordered[right].Line {
			return ordered[left].Line < ordered[right].Line
		}
		if ordered[left].EndLine != ordered[right].EndLine {
			return ordered[left].EndLine < ordered[right].EndLine
		}
		if ordered[left].Evidence != ordered[right].Evidence {
			return ordered[left].Evidence < ordered[right].Evidence
		}
		return ordered[left].ID.String() < ordered[right].ID.String()
	})
	return ordered
}
