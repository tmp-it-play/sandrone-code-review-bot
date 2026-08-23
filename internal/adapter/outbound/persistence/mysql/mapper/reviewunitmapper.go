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
		Ordinal:          unit.Ordinal,
		Kind:             unit.Kind,
		Status:           string(unit.Status),
		AttemptCount:     unit.AttemptCount,
		Provider:         unit.Provider,
		Model:            unit.Model,
		PromptTokens:     unit.PromptTokens,
		CompletionTokens: unit.CompletionTokens,
		TotalTokens:      unit.TotalTokens,
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
		Ordinal:          entry.Ordinal,
		Kind:             entry.Kind,
		Status:           reviewworkflow.UnitStatus(entry.Status),
		AttemptCount:     entry.AttemptCount,
		Provider:         entry.Provider,
		Model:            entry.Model,
		PromptTokens:     entry.PromptTokens,
		CompletionTokens: entry.CompletionTokens,
		TotalTokens:      entry.TotalTokens,
		ErrorSummary:     entry.ErrorSummary,
		StartedAt:        entry.StartedAt,
		FinishedAt:       entry.FinishedAt,
		HeartbeatAt:      entry.HeartbeatAt,
		LeaseToken:       entry.LeaseToken,
		LeaseExpiresAt:   entry.LeaseExpiresAt,
	}
}
