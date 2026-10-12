package reviewsummary

import (
	"encoding/json"
	"strings"
)

func CanonicalOverview(value string) (string, error) {
	value = strings.TrimSpace(value)
	sentences := make([]string, 0, MaxSentences)
	start := 0
	for _, match := range sentenceEnding.FindAllStringIndex(value, -1) {
		sentences = append(sentences, strings.TrimSpace(value[start:match[1]]))
		start = match[1]
	}
	if tail := strings.TrimSpace(value[start:]); tail != "" {
		sentences = append(sentences, tail)
	}
	encoded, err := json.Marshal(Overview{Sentences: sentences})
	if err != nil {
		return "", err
	}
	return Parse(string(encoded))
}
