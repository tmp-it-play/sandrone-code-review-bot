package reviewanalysis

import (
	"sort"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/diff"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type EvidenceVerifier struct{}

func (v EvidenceVerifier) Verify(findings []review.Finding, files []pullrequest.ChangedFile) ([]review.Finding, int) {
	verified, report := v.VerifyWithReport(findings, files)
	return verified, report.Dropped()
}

func (v EvidenceVerifier) VerifyWithReport(findings []review.Finding, files []pullrequest.ChangedFile) ([]review.Finding, EvidenceReport) {
	addedByFile := make(map[string]map[int]string, len(files))
	for _, file := range files {
		addedByFile[file.Path] = parseAddedLines(file.Patch)
	}
	verified := make([]review.Finding, 0, len(findings))
	report := EvidenceReport{}
	for _, finding := range findings {
		lines, ok := addedByFile[finding.File]
		if !ok {
			report.UnknownFile++
			continue
		}
		resolved, outcome := resolveEvidence(finding, lines)
		switch outcome {
		case "exact":
			report.Exact++
		case "normalized":
			report.Normalized++
		case "reanchored":
			report.Reanchored++
		case "invalid_span":
			report.InvalidSpan++
			continue
		case "non_added_span":
			report.NonAddedSpan++
			continue
		case "ambiguous":
			report.Ambiguous++
			continue
		default:
			report.NotFound++
			continue
		}
		verified = append(verified, resolved.WithIdentity())
	}
	return verified, report
}

func parseAddedLines(patch string) map[int]string {
	lines := map[int]string{}
	for _, hunk := range (diff.Parser{}).Parse(patch) {
		if !hunk.Complete {
			continue
		}
		current := hunk.NewStart
		for _, raw := range strings.Split(hunk.Body, "\n") {
			switch {
			case strings.HasPrefix(raw, "+"):
				lines[current] = raw[1:]
				current++
			case strings.HasPrefix(raw, " "):
				current++
			}
		}
	}
	return lines
}

func resolveEvidence(finding review.Finding, lines map[int]string) (review.Finding, string) {
	originalEvidence := finding.Evidence
	variantGroups := evidenceVariantGroups(originalEvidence)
	actual, claimed := evidenceAt(lines, finding.Line, finding.EndLine)
	if claimed {
		for _, variants := range variantGroups {
			for _, variant := range variants {
				if variant != actual {
					continue
				}
				finding.Evidence = actual
				if originalEvidence == actual {
					return finding, "exact"
				}
				return finding, "normalized"
			}
		}
	}
	if !finding.Snapped {
		for _, variants := range variantGroups {
			matches := evidenceMatches(lines, variants)
			if len(matches) == 0 {
				continue
			}
			if len(matches) > 1 {
				matches = matchesAtClaimedLine(matches, finding.Line)
			}
			if len(matches) > 1 {
				return review.Finding{}, "ambiguous"
			}
			finding.Line = matches[0][0]
			finding.EndLine = matches[0][1]
			if finding.EndLine == finding.Line {
				finding.EndLine = 0
			}
			finding.Evidence, _ = evidenceAt(lines, matches[0][0], matches[0][1])
			finding.Snapped = true
			return finding, "reanchored"
		}
	}
	if !validEvidenceSpan(finding.Line, finding.EndLine) {
		return review.Finding{}, "invalid_span"
	}
	if !claimed {
		return review.Finding{}, "non_added_span"
	}
	return review.Finding{}, "not_found"
}

func validEvidenceSpan(start int, end int) bool {
	if end == 0 {
		end = start
	}
	return start > 0 && end >= start && end-start <= 50
}

func evidenceAt(lines map[int]string, start int, end int) (string, bool) {
	if end == 0 {
		end = start
	}
	if !validEvidenceSpan(start, end) {
		return "", false
	}
	actual := make([]string, 0, end-start+1)
	for line := start; line <= end; line++ {
		content, ok := lines[line]
		if !ok {
			return "", false
		}
		actual = append(actual, content)
	}
	return strings.Join(actual, "\n"), true
}

func evidenceVariantGroups(value string) [][]string {
	normalized := strings.ReplaceAll(value, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	candidates := []string{normalized}
	if strings.HasSuffix(normalized, "\n") {
		candidates = append(candidates, strings.TrimSuffix(normalized, "\n"))
	}
	seen := map[string]struct{}{}
	groups := make([][]string, 0, len(candidates)*2)
	for _, variant := range candidates {
		if strings.TrimSpace(variant) == "" {
			continue
		}
		if _, exists := seen[variant]; exists {
			continue
		}
		seen[variant] = struct{}{}
		groups = append(groups, []string{variant})
	}
	for _, variant := range candidates {
		stripped, ok := stripAddedPrefixes(variant)
		if !ok || strings.TrimSpace(stripped) == "" {
			continue
		}
		if _, exists := seen[stripped]; exists {
			continue
		}
		seen[stripped] = struct{}{}
		groups = append(groups, []string{stripped})
	}
	return groups
}

func stripAddedPrefixes(value string) (string, bool) {
	lines := strings.Split(value, "\n")
	for index := range lines {
		if !strings.HasPrefix(lines[index], "+") {
			return "", false
		}
		lines[index] = strings.TrimPrefix(lines[index], "+")
	}
	return strings.Join(lines, "\n"), true
}

func evidenceMatches(lines map[int]string, variants []string) [][2]int {
	starts := make([]int, 0, len(lines))
	for line := range lines {
		starts = append(starts, line)
	}
	sort.Ints(starts)
	matches := map[[2]int]struct{}{}
	for _, variant := range variants {
		lineCount := len(strings.Split(variant, "\n"))
		if lineCount < 1 || lineCount > 51 {
			continue
		}
		for _, start := range starts {
			end := start + lineCount - 1
			actual, ok := evidenceAt(lines, start, end)
			if ok && actual == variant {
				matches[[2]int{start, end}] = struct{}{}
			}
		}
	}
	ordered := make([][2]int, 0, len(matches))
	for match := range matches {
		ordered = append(ordered, match)
	}
	sort.Slice(ordered, func(left int, right int) bool {
		if ordered[left][0] != ordered[right][0] {
			return ordered[left][0] < ordered[right][0]
		}
		return ordered[left][1] < ordered[right][1]
	})
	return ordered
}

func matchesAtClaimedLine(matches [][2]int, line int) [][2]int {
	selected := make([][2]int, 0, 1)
	for _, match := range matches {
		if match[0] == line {
			selected = append(selected, match)
		}
	}
	if len(selected) == 1 {
		return selected
	}
	return matches
}
