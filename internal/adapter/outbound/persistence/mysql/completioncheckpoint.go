package mysql

import (
	"encoding/hex"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
)

func aggregateCompletionAttempt(prior publication.CompletionCheckpoint, response llm.Response, multipleModels bool) publication.CompletionCheckpoint {
	priorIdentity := prior.Provider != "" || prior.Model != ""
	responseIdentity := response.Provider != "" || response.Model != ""
	multiple := prior.MultipleModels || multipleModels || response.ModelLabel == "복수 모델" || response.Provider == "multiple" || response.Model == "multiple"
	if priorIdentity && responseIdentity && (prior.Provider != response.Provider || prior.Model != response.Model) {
		multiple = true
	}
	if response.Provider != "" {
		prior.Provider = response.Provider
	}
	if response.Model != "" {
		prior.Model = response.Model
	}
	if response.ModelLabel != "" {
		prior.ModelLabel = response.ModelLabel
	}
	prior.MultipleModels = multiple
	if multiple {
		prior.ModelLabel = "복수 모델"
	}
	prior.Usage = normalizedCompletionUsage(prior.Usage).Add(normalizedCompletionUsage(response.Usage))
	prior.ToolExecutions += response.ToolExecutions
	return prior
}

func normalizedCompletionUsage(value llm.Usage) llm.Usage {
	if value.TotalTokens == 0 && (value.PromptTokens > 0 || value.CompletionTokens > 0) {
		value.TotalTokens = value.PromptTokens + value.CompletionTokens
	}
	return value
}

func validCompletionAttempt(response llm.Response) bool {
	return response.Usage.PromptTokens >= 0 && response.Usage.CompletionTokens >= 0 && response.Usage.TotalTokens >= 0 && response.ToolExecutions >= 0
}

func validCompletionCheckpoint(checkpoint publication.CompletionCheckpoint) bool {
	if len(checkpoint.InputHash) != 64 || strings.TrimSpace(checkpoint.CanonicalContent) == "" || checkpoint.CompletedAt.IsZero() {
		return false
	}
	if _, err := hex.DecodeString(checkpoint.InputHash); err != nil {
		return false
	}
	if checkpoint.Provider == "" || checkpoint.Model == "" || !validCompletionAttempt(checkpoint.MetadataResponse()) {
		return false
	}
	return checkpoint.MetadataResponse().Completed()
}
