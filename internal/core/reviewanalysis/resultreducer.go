package reviewanalysis

import (
	"regexp"
	"sort"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

const maxMergedOverviewChars = 6000
const maxMergedFileNotes = 40

var internalReviewUnitLabel = regexp.MustCompile(`(?i)(?:#{1,6}\s*|\*{1,2})?(?:(?:리뷰|검토)\s*(?:단위|배치)|배치|review\s*(?:unit|batch))\s*#?\d+(?:\s*[/／]\s*\d+)?\s*\*{0,2}\s*[:：.\-–—]*`)

type ResultReducer struct{}

func (r ResultReducer) Reduce(results []review.Result) (review.Result, int) {
	merged := review.Result{}
	fileNotes := map[string]string{}
	overviews := make([]string, 0, len(results))
	seenOverviews := map[string]struct{}{}
	for _, result := range results {
		overview := normalizeOverview(result.Summary.Overview)
		if overview != "" {
			if _, exists := seenOverviews[overview]; !exists {
				seenOverviews[overview] = struct{}{}
				overviews = append(overviews, overview)
			}
		}
		for _, file := range result.Summary.Files {
			path := strings.TrimSpace(file.Path)
			note := strings.TrimSpace(file.Note)
			if path == "" {
				continue
			}
			if current, ok := fileNotes[path]; !ok || preferNote(note, current) {
				fileNotes[path] = note
			}
		}
		merged.Findings = append(merged.Findings, result.Findings...)
	}
	merged.Summary.Overview = mergedOverview(overviews)
	paths := make([]string, 0, len(fileNotes))
	for path := range fileNotes {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if len(merged.Summary.Files) >= maxMergedFileNotes {
			break
		}
		merged.Summary.Files = append(merged.Summary.Files, review.FileNote{Path: path, Note: fileNotes[path]})
	}
	return merged, 0
}

func normalizeOverview(value string) string {
	withoutLabels := internalReviewUnitLabel.ReplaceAllString(value, "")
	return strings.Join(strings.Fields(withoutLabels), " ")
}

func mergedOverview(overviews []string) string {
	if len(overviews) == 0 {
		return ""
	}
	if len(overviews) == 1 {
		return boundedOverview(overviews[0])
	}
	return boundedOverview(strings.Join(overviews, " "))
}

func boundedOverview(value string) string {
	characters := []rune(value)
	if len(characters) <= maxMergedOverviewChars {
		return value
	}
	return strings.TrimSpace(string(characters[:maxMergedOverviewChars])) + "…"
}

func preferNote(candidate string, current string) bool {
	if len(candidate) != len(current) {
		return len(candidate) > len(current)
	}
	return candidate < current
}
