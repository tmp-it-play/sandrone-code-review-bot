package prompt

import "strings"

const MaxBodyChars = 4000

func truncateBody(body string) (string, bool) {
	trimmed := strings.TrimSpace(body)
	if len(trimmed) <= MaxBodyChars {
		return trimmed, false
	}
	cut := trimmed[:MaxBodyChars]
	if boundary := strings.LastIndex(cut, "\n"); boundary > MaxBodyChars/2 {
		cut = cut[:boundary]
	}
	return strings.TrimSpace(cut), true
}

func BodyCost(body string) int {
	if len(body) > MaxBodyChars {
		return MaxBodyChars
	}
	return len(body)
}
