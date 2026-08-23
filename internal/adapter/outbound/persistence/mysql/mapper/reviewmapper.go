package mapper

import (
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

func ToReviewModel(record review.Record) model.Review {
	var reviewRunID *uint64
	if record.RunID != 0 {
		reviewRunID = &record.RunID
	}
	return model.Review{
		ID:            record.ID,
		ReviewRunID:   reviewRunID,
		Owner:         record.Owner,
		Repository:    record.Repository,
		Number:        record.Number,
		HeadSHA:       record.HeadSHA,
		Trigger:       string(record.Trigger),
		Outcome:       string(record.Outcome),
		Provider:      record.Provider,
		Model:         record.Model,
		InlineCount:   record.InlineCount,
		FallbackCount: record.FallbackCount,
		Detail:        record.Detail,
		StartedAt:     record.StartedAt,
		FinishedAt:    record.FinishedAt,
	}
}

func ToReviewRecord(entry model.Review) review.Record {
	runID := uint64(0)
	if entry.ReviewRunID != nil {
		runID = *entry.ReviewRunID
	}
	return review.Record{
		ID:            entry.ID,
		RunID:         runID,
		Owner:         entry.Owner,
		Repository:    entry.Repository,
		Number:        entry.Number,
		HeadSHA:       entry.HeadSHA,
		Trigger:       review.Trigger(entry.Trigger),
		Outcome:       review.Outcome(entry.Outcome),
		Provider:      entry.Provider,
		Model:         entry.Model,
		InlineCount:   entry.InlineCount,
		FallbackCount: entry.FallbackCount,
		Detail:        entry.Detail,
		StartedAt:     entry.StartedAt,
		FinishedAt:    entry.FinishedAt,
	}
}
