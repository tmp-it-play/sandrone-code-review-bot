package mysql

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func storedUnitResult(unit model.ReviewUnit) (reviewworkflow.UnitResult, error) {
	result := review.Result{}
	if unit.ResultJSON != "" {
		if err := json.Unmarshal([]byte(unit.ResultJSON), &result); err != nil {
			return reviewworkflow.UnitResult{}, fmt.Errorf("저장된 리뷰 unit 결과를 읽지 못했습니다: %w", err)
		}
	}
	finishedAt := time.Time{}
	if unit.FinishedAt != nil {
		finishedAt = *unit.FinishedAt
	}
	retryAt := time.Time{}
	if unit.RetryAt != nil {
		retryAt = *unit.RetryAt
	}
	return reviewworkflow.UnitResult{
		Status:         reviewworkflow.UnitStatus(unit.Status),
		InputHash:      unit.InputHash,
		Provider:       unit.Provider,
		Model:          unit.Model,
		MultipleModels: unit.MultipleModels,
		Usage:          llm.Usage{PromptTokens: unit.PromptTokens, CompletionTokens: unit.CompletionTokens, TotalTokens: unit.TotalTokens},
		Review:         result,
		ToolExecutions: unit.ToolExecutions,
		Reused:         unit.Reused,
		Retryable:      unit.Retryable,
		RetryAt:        retryAt,
		FinishedAt:     finishedAt,
	}, nil
}
