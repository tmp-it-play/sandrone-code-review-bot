package mapper

import (
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func ToReviewRunModel(run reviewworkflow.Run) model.ReviewRun {
	return model.ReviewRun{
		ID:                 run.ID,
		RunKey:             run.Key,
		InstallationID:     run.InstallationID,
		Owner:              run.Owner,
		Repository:         run.Repository,
		Number:             run.Number,
		BaseSHA:            run.BaseSHA,
		HeadSHA:            run.HeadSHA,
		ConfigHash:         run.ConfigHash,
		PromptVersion:      run.PromptVersion,
		ModelPolicyHash:    run.ModelPolicyHash,
		RequestIdentity:    run.RequestIdentity,
		Trigger:            string(run.Trigger),
		Status:             string(run.Status),
		SnapshotObservedAt: run.SnapshotObservedAt,
		SnapshotOrderKey:   run.SnapshotOrderKey,
		TotalCoverage:      run.TotalCoverage,
		ReviewedCoverage:   run.ReviewedCoverage,
		FailedCoverage:     run.FailedCoverage,
		DeferredCoverage:   run.DeferredCoverage,
		SkippedCoverage:    run.SkippedCoverage,
		ExternalCalls:      run.ExternalCalls,
		ErrorSummary:       run.ErrorSummary,
		SupersedingHeadSHA: run.SupersedingHeadSHA,
		StartedAt:          run.StartedAt,
		HeartbeatAt:        run.HeartbeatAt,
		TerminalAt:         run.TerminalAt,
		ExpiresAt:          run.ExpiresAt,
		LeaseToken:         run.LeaseToken,
		LeaseExpiresAt:     run.LeaseExpiresAt,
	}
}

func ToReviewRun(entry model.ReviewRun) reviewworkflow.Run {
	return reviewworkflow.Run{
		ID:                 entry.ID,
		Key:                entry.RunKey,
		InstallationID:     entry.InstallationID,
		Owner:              entry.Owner,
		Repository:         entry.Repository,
		Number:             entry.Number,
		BaseSHA:            entry.BaseSHA,
		HeadSHA:            entry.HeadSHA,
		ConfigHash:         entry.ConfigHash,
		PromptVersion:      entry.PromptVersion,
		ModelPolicyHash:    entry.ModelPolicyHash,
		RequestIdentity:    entry.RequestIdentity,
		Trigger:            review.Trigger(entry.Trigger),
		Status:             reviewworkflow.RunStatus(entry.Status),
		SnapshotObservedAt: entry.SnapshotObservedAt,
		SnapshotOrderKey:   entry.SnapshotOrderKey,
		TotalCoverage:      entry.TotalCoverage,
		ReviewedCoverage:   entry.ReviewedCoverage,
		FailedCoverage:     entry.FailedCoverage,
		DeferredCoverage:   entry.DeferredCoverage,
		SkippedCoverage:    entry.SkippedCoverage,
		ExternalCalls:      entry.ExternalCalls,
		ErrorSummary:       entry.ErrorSummary,
		SupersedingHeadSHA: entry.SupersedingHeadSHA,
		StartedAt:          entry.StartedAt,
		HeartbeatAt:        entry.HeartbeatAt,
		TerminalAt:         entry.TerminalAt,
		ExpiresAt:          entry.ExpiresAt,
		LeaseToken:         entry.LeaseToken,
		LeaseExpiresAt:     entry.LeaseExpiresAt,
	}
}
