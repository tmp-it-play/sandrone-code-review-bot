package mapper

import (
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func ToCoverageItemModel(item reviewworkflow.CoverageItem) model.CoverageItem {
	return model.CoverageItem{
		ID:               item.ID,
		ReviewRunID:      item.RunID,
		ReviewUnitID:     item.UnitID,
		CoverageKey:      item.Key,
		Kind:             string(item.Kind),
		Path:             item.Path,
		PreviousPath:     item.PreviousPath,
		FileStatus:       item.FileStatus,
		HunkHash:         item.HunkHash,
		DuplicateOrdinal: item.DuplicateOrdinal,
		OldStart:         item.OldStart,
		OldCount:         item.OldCount,
		NewStart:         item.NewStart,
		NewCount:         item.NewCount,
		Eligibility:      string(item.Eligibility),
		Status:           string(item.Status),
		Reason:           item.Reason,
		ReviewedAt:       item.ReviewedAt,
	}
}

func ToCoverageItem(entry model.CoverageItem) reviewworkflow.CoverageItem {
	return reviewworkflow.CoverageItem{
		ID:               entry.ID,
		RunID:            entry.ReviewRunID,
		UnitID:           entry.ReviewUnitID,
		Key:              entry.CoverageKey,
		Kind:             reviewworkflow.CoverageKind(entry.Kind),
		Path:             entry.Path,
		PreviousPath:     entry.PreviousPath,
		FileStatus:       entry.FileStatus,
		HunkHash:         entry.HunkHash,
		DuplicateOrdinal: entry.DuplicateOrdinal,
		OldStart:         entry.OldStart,
		OldCount:         entry.OldCount,
		NewStart:         entry.NewStart,
		NewCount:         entry.NewCount,
		Eligibility:      reviewworkflow.CoverageEligibility(entry.Eligibility),
		Status:           reviewworkflow.CoverageStatus(entry.Status),
		Reason:           entry.Reason,
		ReviewedAt:       entry.ReviewedAt,
	}
}
