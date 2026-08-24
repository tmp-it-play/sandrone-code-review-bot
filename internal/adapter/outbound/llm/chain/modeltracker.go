package chain

import (
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
)

type completionModelTracker struct {
	models map[string]struct{}
}

func newCompletionModelTracker() *completionModelTracker {
	return &completionModelTracker{models: map[string]struct{}{}}
}

func (t *completionModelTracker) Add(response llm.Response, fallback string) llm.Response {
	model := strings.TrimSpace(response.Model)
	if model == "" {
		model = fallback
		response.Model = model
	}
	if model != "" {
		t.models[model] = struct{}{}
	}
	return response
}

func (t *completionModelTracker) Apply(response llm.Response) llm.Response {
	if len(t.models) > 1 {
		response.Model = "multiple"
		response.ModelLabel = "복수 모델"
	}
	return response
}
