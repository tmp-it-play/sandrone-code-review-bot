package review

import "strings"

func OccurrenceEvidencePresent(content string, evidence string) bool {
	return len(OccurrenceEvidenceLines(content, evidence)) > 0
}

func OccurrenceEvidenceLines(content string, evidence string) []int {
	content = strings.TrimSuffix(normalizeOccurrenceLineEndings(content), "\n")
	evidence = strings.TrimSuffix(normalizeOccurrenceLineEndings(evidence), "\n")
	if evidence == "" {
		return nil
	}
	contentLines := strings.Split(content, "\n")
	evidenceLines := strings.Split(evidence, "\n")
	if len(evidenceLines) > len(contentLines) {
		return nil
	}
	matches := make([]int, 0, 1)
	for start := 0; start+len(evidenceLines) <= len(contentLines); start++ {
		matched := true
		for offset := range evidenceLines {
			if contentLines[start+offset] != evidenceLines[offset] {
				matched = false
				break
			}
		}
		if matched {
			matches = append(matches, start+1)
		}
	}
	return matches
}

func OccurrenceEvidenceLineCount(evidence string) int {
	evidence = strings.TrimSuffix(normalizeOccurrenceLineEndings(evidence), "\n")
	if evidence == "" {
		return 0
	}
	return len(strings.Split(evidence, "\n"))
}

func normalizeOccurrenceLineEndings(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.ReplaceAll(value, "\r", "\n")
}
