package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *ReviewRunStore) HasRegisteredReviewContinuation(ctx context.Context, run reviewworkflow.Run, activityBoundary time.Time) (bool, error) {
	var observedState model.PullRequestState
	result := r.database.WithContext(ctx).
		Select("latest_run_id", "latest_head_sha", "latest_run_at").
		Where("owner = ? AND repository = ? AND number = ?", run.Owner, run.Repository, run.Number).
		Take(&observedState)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if result.Error != nil {
		return false, fmt.Errorf("등록된 최신 리뷰 실행을 읽지 못했습니다: %w", result.Error)
	}
	if observedState.LatestRunID == 0 {
		return false, nil
	}
	if observedState.LatestHeadSHA == "" {
		return false, nil
	}
	if observedState.LatestRunAt == nil {
		return false, nil
	}
	observedLatestRunAt := *observedState.LatestRunAt
	continuation := false
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var registeredRun model.ReviewRun
		result := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&registeredRun, observedState.LatestRunID)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil
		}
		if result.Error != nil {
			return result.Error
		}
		var lockedState model.PullRequestState
		result = transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("owner = ? AND repository = ? AND number = ?", run.Owner, run.Repository, run.Number).
			Take(&lockedState)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil
		}
		if result.Error != nil {
			return result.Error
		}
		if lockedState.LatestRunID != observedState.LatestRunID || lockedState.LatestHeadSHA != observedState.LatestHeadSHA {
			return nil
		}
		if lockedState.LatestRunAt == nil || !lockedState.LatestRunAt.Equal(observedLatestRunAt) {
			return nil
		}
		if registeredRun.Owner != run.Owner || registeredRun.Repository != run.Repository || registeredRun.Number != run.Number {
			return nil
		}
		if registeredRun.HeadSHA != lockedState.LatestHeadSHA {
			return nil
		}
		if registeredRun.ID == run.ID {
			if registeredRun.HeadSHA != run.HeadSHA || reviewworkflow.RunStatus(registeredRun.Status).IsTerminal() {
				return nil
			}
			continuation = true
			return nil
		}
		if registeredRun.SnapshotObservedAt.After(run.SnapshotObservedAt) {
			return nil
		}
		hasPendingHeadReplacement := reviewworkflow.RunStatus(registeredRun.Status) == reviewworkflow.RunStatusSuperseded && registeredRun.SupersedingHeadSHA != "" && registeredRun.SupersedingHeadSHA != registeredRun.HeadSHA
		if registeredRun.HeadSHA == run.HeadSHA && !hasPendingHeadReplacement {
			return nil
		}
		if !hasPendingHeadReplacement {
			if lockedState.LatestRunAt.After(activityBoundary) {
				return nil
			}
			if registeredRun.TerminalAt != nil && !registeredRun.TerminalAt.After(activityBoundary) {
				return nil
			}
		}
		continuation = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("등록된 리뷰 실행의 연속성을 확인하지 못했습니다: %w", err)
	}
	return continuation, nil
}
