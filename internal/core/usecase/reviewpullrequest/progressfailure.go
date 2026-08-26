package reviewpullrequest

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func (u *UseCase) finishUnleasedProgressFailure(ctx context.Context, task job.ReviewJob, startedAt time.Time, detail string, runID uint64, progress *progressSession) (uint64, reviewworkflow.RunStatus, error) {
	progress.Stop()
	persistenceContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*reactionTimeout)
	defer cancel()
	finalization := progress.TerminalFinalization()
	if finalization == nil {
		return 0, "", errors.New("진행 코멘트 종료 marker가 없습니다")
	}
	detachedFinalization := runID == 0
	terminalAt := u.deps.Clock.Now()
	expiresAt := terminalAt.Add(u.deps.Retention)
	if detachedFinalization {
		key, valid := reviewworkflow.ProgressMarkerKey(finalization.Marker)
		if !valid {
			return 0, "", errors.New("진행 코멘트 종료 marker가 올바르지 않습니다")
		}
		observedAt := task.SnapshotObservedAt
		if observedAt.IsZero() {
			observedAt = task.RequestReceivedAt
		}
		if observedAt.IsZero() {
			observedAt = startedAt
		}
		orderKey := strings.TrimSpace(task.SnapshotOrderKey)
		if orderKey == "" {
			orderKey = "progress:" + key
		}
		runID, status, err := u.deps.Runs.FinishDetachedProgressFailure(persistenceContext, reviewworkflow.Run{
			Key:                key,
			InstallationID:     task.Target.InstallationID,
			Owner:              task.Target.Owner,
			Repository:         task.Target.Repository,
			Number:             task.Target.Number,
			BaseSHA:            task.Target.BaseSHA,
			HeadSHA:            task.Target.HeadSHA,
			RequestIdentity:    task.RequestIdentity,
			ProgressMarker:     finalization.Marker,
			Trigger:            task.Trigger,
			Status:             reviewworkflow.RunStatusPlanning,
			SnapshotObservedAt: observedAt,
			SnapshotOrderKey:   orderKey,
			StartedAt:          startedAt,
			HeartbeatAt:        startedAt,
		}, reviewworkflow.RunResult{
			Status:               reviewworkflow.RunStatusFailed,
			ReviewOutcome:        review.OutcomeFailed,
			Error:                detail,
			TerminalAt:           terminalAt,
			ExpiresAt:            expiresAt,
			DetachedFinalization: true,
			TerminalProgress:     finalization,
		})
		if err != nil {
			return 0, "", err
		}
		progress.SetRunID(runID)
		return runID, status, nil
	}
	progress.SetRunID(runID)
	status, err := u.deps.Runs.FinishRun(persistenceContext, runID, reviewworkflow.RunResult{
		Status:               reviewworkflow.RunStatusFailed,
		ReviewOutcome:        review.OutcomeFailed,
		Error:                detail,
		TerminalAt:           terminalAt,
		ExpiresAt:            expiresAt,
		DetachedFinalization: false,
		TerminalProgress:     finalization,
	})
	if err != nil {
		return 0, "", err
	}
	return runID, status, nil
}
