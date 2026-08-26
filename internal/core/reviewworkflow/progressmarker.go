package reviewworkflow

import "strings"

const publicationMarkerPrefix = "<!-- sandrone-review-run:"
const progressMarkerPrefix = "<!-- sandrone-review-progress:"

func ProgressMarker(runKey string) string {
	return progressMarkerPrefix + runKey + " -->"
}

func ProgressMarkerForPublicationMarker(marker string) (string, bool) {
	if !strings.HasPrefix(marker, publicationMarkerPrefix) {
		return "", false
	}
	return progressMarkerPrefix + strings.TrimPrefix(marker, publicationMarkerPrefix), true
}

func ProgressMarkers(body string, publicationMarker string) []string {
	trimmed := strings.TrimSpace(body)
	if !strings.HasSuffix(trimmed, publicationMarker) {
		return nil
	}
	prefix := strings.TrimSpace(strings.TrimSuffix(trimmed, publicationMarker))
	lines := strings.Split(prefix, "\n")
	markers := make([]string, 0, 2)
	seen := make(map[string]struct{})
	for index := len(lines) - 1; index >= 0 && len(lines)-index <= 2; index-- {
		marker := strings.TrimSpace(lines[index])
		if !validProgressMarker(marker) {
			break
		}
		if _, exists := seen[marker]; !exists {
			seen[marker] = struct{}{}
			markers = append([]string{marker}, markers...)
		}
	}
	return markers
}

func validProgressMarker(marker string) bool {
	_, valid := ProgressMarkerKey(marker)
	return valid
}

func ProgressMarkerKey(marker string) (string, bool) {
	if !strings.HasPrefix(marker, progressMarkerPrefix) || !strings.HasSuffix(marker, " -->") {
		return "", false
	}
	key := strings.TrimSuffix(strings.TrimPrefix(marker, progressMarkerPrefix), " -->")
	if len(key) != 64 {
		return "", false
	}
	for _, character := range key {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return "", false
		}
	}
	return key, true
}
