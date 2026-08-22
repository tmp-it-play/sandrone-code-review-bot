package parsing

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

var ErrNoPayload = errors.New("모델 응답에서 JSON을 찾지 못했다")

type ResultParser struct{}

func (p ResultParser) Parse(raw string) (review.Result, error) {
	payload, ok := extractObject(raw)
	if !ok {
		return review.Result{}, ErrNoPayload
	}
	var decoded resultPayload
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		return review.Result{}, err
	}
	return decoded.toDomain(), nil
}

func extractObject(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if fenced, ok := stripFence(trimmed); ok {
		trimmed = fenced
	}
	start := strings.Index(trimmed, "{")
	if start < 0 {
		return "", false
	}
	depth := 0
	inString := false
	escaped := false
	for index := start; index < len(trimmed); index++ {
		symbol := trimmed[index]
		switch {
		case escaped:
			escaped = false
		case symbol == '\\' && inString:
			escaped = true
		case symbol == '"':
			inString = !inString
		case inString:
		case symbol == '{':
			depth++
		case symbol == '}':
			depth--
			if depth == 0 {
				return trimmed[start : index+1], true
			}
		}
	}
	return "", false
}

func stripFence(text string) (string, bool) {
	if !strings.HasPrefix(text, "```") {
		return "", false
	}
	rest := text[3:]
	if newline := strings.Index(rest, "\n"); newline >= 0 {
		rest = rest[newline+1:]
	}
	if end := strings.LastIndex(rest, "```"); end >= 0 {
		rest = rest[:end]
	}
	return strings.TrimSpace(rest), true
}
