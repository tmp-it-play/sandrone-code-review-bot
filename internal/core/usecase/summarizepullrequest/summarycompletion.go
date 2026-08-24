package summarizepullrequest

import (
	"encoding/json"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type summaryCompletionPayload struct {
	Version string         `json:"version"`
	Summary review.Summary `json:"summary"`
}

func canonicalSummaryContent(summary review.Summary) (string, error) {
	encoded, err := json.Marshal(summaryCompletionPayload{Version: "summary-completion-v1", Summary: summary})
	if err != nil {
		return "", fmt.Errorf("요약 완료 결과를 직렬화하지 못했습니다: %w", err)
	}
	return string(encoded), nil
}

func summaryFromCanonicalContent(content string) (review.Summary, error) {
	var payload summaryCompletionPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return review.Summary{}, fmt.Errorf("요약 완료 결과를 역직렬화하지 못했습니다: %w", err)
	}
	if payload.Version != "summary-completion-v1" || payload.Summary.IsEmpty() {
		return review.Summary{}, fmt.Errorf("요약 완료 결과 형식이 올바르지 않습니다")
	}
	return payload.Summary, nil
}
