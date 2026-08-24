package reviewanalysis

import (
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/diff"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type EvidenceVerifier struct{}

func (v EvidenceVerifier) Verify(findings []review.Finding, files []pullrequest.ChangedFile) ([]review.Finding, int) {
	addedByFile := make(map[string]map[int]string, len(files))
	for _, file := range files {
		addedByFile[file.Path] = parseAddedLines(file.Patch)
	}
	verified := make([]review.Finding, 0, len(findings))
	dropped := 0
	for _, finding := range findings {
		lines, ok := addedByFile[finding.File]
		if !ok || !evidenceMatches(finding, lines) {
			dropped++
			continue
		}
		verified = append(verified, finding.WithIdentity())
	}
	return verified, dropped
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

func evidenceMatches(finding review.Finding, lines map[int]string) bool {
	start := finding.Line
	end := finding.EndLine
	if end == 0 {
		end = start
	}
	if start <= 0 || end < start || end-start > 50 || strings.TrimSpace(finding.Evidence) == "" {
		return false
	}
	actual := make([]string, 0, end-start+1)
	for line := start; line <= end; line++ {
		content, ok := lines[line]
		if !ok {
			return false
		}
		actual = append(actual, content)
	}
	return finding.Evidence == strings.Join(actual, "\n")
}
