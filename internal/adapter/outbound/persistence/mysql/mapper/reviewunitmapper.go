package mapper

import (
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func ToReviewUnitModel(unit reviewworkflow.Unit) model.ReviewUnit {
	return model.ReviewUnit{
		ID:               unit.ID,
		ReviewRunID:      unit.RunID,
		UnitHash:         unit.Hash,
		ParentUnitHash:   unit.ParentHash,
		InputHash:        unit.InputHash,
		Ordinal:          unit.Ordinal,
		Depth:            unit.Depth,
		OrderKey:         unit.OrderKey,
		Kind:             unit.Kind,
		SpecJSON:         unit.SpecJSON,
		SplitHash:        unit.SplitHash,
		Status:           string(unit.Status),
		AttemptCount:     unit.AttemptCount,
		Provider:         unit.Provider,
		Model:            unit.Model,
		MultipleModels:   unit.MultipleModels,
		PromptTokens:     unit.PromptTokens,
		CompletionTokens: unit.CompletionTokens,
		TotalTokens:      unit.TotalTokens,
		ToolExecutions:   unit.ToolExecutions,
		ResultJSON:       unit.ResultJSON,
		Reused:           unit.Reused,
		Retryable:        unit.Retryable,
		RetryAt:          unit.RetryAt,
		ErrorSummary:     unit.ErrorSummary,
		StartedAt:        unit.StartedAt,
		FinishedAt:       unit.FinishedAt,
		HeartbeatAt:      unit.HeartbeatAt,
		LeaseToken:       unit.LeaseToken,
		LeaseExpiresAt:   unit.LeaseExpiresAt,
	}
}

func ToReviewUnit(entry model.ReviewUnit) reviewworkflow.Unit {
	return reviewworkflow.Unit{
		ID:               entry.ID,
		RunID:            entry.ReviewRunID,
		Hash:             entry.UnitHash,
		ParentHash:       entry.ParentUnitHash,
		InputHash:        entry.InputHash,
		Ordinal:          entry.Ordinal,
		Depth:            entry.Depth,
		OrderKey:         entry.OrderKey,
		Kind:             entry.Kind,
		SpecJSON:         entry.SpecJSON,
		SplitHash:        entry.SplitHash,
		Status:           reviewworkflow.UnitStatus(entry.Status),
		AttemptCount:     entry.AttemptCount,
		Provider:         entry.Provider,
		Model:            entry.Model,
		MultipleModels:   entry.MultipleModels,
		PromptTokens:     entry.PromptTokens,
		CompletionTokens: entry.CompletionTokens,
		TotalTokens:      entry.TotalTokens,
		ToolExecutions:   entry.ToolExecutions,
		ResultJSON:       entry.ResultJSON,
		Reused:           entry.Reused,
		Retryable:        entry.Retryable,
		RetryAt:          entry.RetryAt,
		ErrorSummary:     entry.ErrorSummary,
		StartedAt:        entry.StartedAt,
		FinishedAt:       entry.FinishedAt,
		HeartbeatAt:      entry.HeartbeatAt,
		LeaseToken:       entry.LeaseToken,
		LeaseExpiresAt:   entry.LeaseExpiresAt,
	}
}
