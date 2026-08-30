package mysql

import (
	"context"
	"errors"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errLatestRunAnchorChanged = errors.New("최신 리뷰 실행을 읽는 동안 대상 실행이 변경되었습니다")

func (r *ReviewRunStore) LatestRunAnchor(ctx context.Context, target pullrequest.Target) (reviewworkflow.RunAnchor, bool, error) {
	for attempt := 0; attempt < 3; attempt++ {
		anchor, found, err := r.latestRunAnchor(ctx, target)
		if !errors.Is(err, errLatestRunAnchorChanged) {
			return anchor, found, err
		}
	}
	return reviewworkflow.RunAnchor{}, false, fmt.Errorf("최신 리뷰 실행 기준점을 읽지 못했습니다: %w", errLatestRunAnchorChanged)
}

func (r *ReviewRunStore) latestRunAnchor(ctx context.Context, target pullrequest.Target) (reviewworkflow.RunAnchor, bool, error) {
	var observedState model.PullRequestState
	result := r.database.WithContext(ctx).
		Select("latest_run_id", "latest_head_sha").
		Where("owner = ? AND repository = ? AND number = ?", target.Owner, target.Repository, target.Number).
		Take(&observedState)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return reviewworkflow.RunAnchor{}, false, nil
	}
	if result.Error != nil {
		return reviewworkflow.RunAnchor{}, false, fmt.Errorf("최신 리뷰 실행 기준점을 읽지 못했습니다: %w", result.Error)
	}
	if observedState.LatestRunID == 0 {
		return reviewworkflow.RunAnchor{}, false, nil
	}
	anchor := reviewworkflow.RunAnchor{}
	found := false
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var run model.ReviewRun
		result := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, observedState.LatestRunID)
		if result.Error != nil {
			return result.Error
		}
		var state model.PullRequestState
		result = transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("owner = ? AND repository = ? AND number = ?", target.Owner, target.Repository, target.Number).
			Take(&state)
		if result.Error != nil {
			return result.Error
		}
		if state.LatestRunID != observedState.LatestRunID || state.LatestHeadSHA != observedState.LatestHeadSHA {
			return errLatestRunAnchorChanged
		}
		if run.Owner != target.Owner || run.Repository != target.Repository || run.Number != target.Number || run.HeadSHA != state.LatestHeadSHA {
			return errors.New("최신 리뷰 실행과 Pull Request 상태가 일치하지 않습니다")
		}
		status := reviewworkflow.RunStatus(run.Status)
		anchor = reviewworkflow.RunAnchor{
			BaseSHA:            run.BaseSHA,
			HeadSHA:            run.HeadSHA,
			ReplacementPending: status == reviewworkflow.RunStatusSuperseded && run.SupersedingHeadSHA != "" && run.SupersedingHeadSHA != run.HeadSHA,
		}
		found = true
		return nil
	})
	if err != nil {
		return reviewworkflow.RunAnchor{}, false, fmt.Errorf("최신 리뷰 실행 기준점을 읽지 못했습니다: %w", err)
	}
	return anchor, found, nil
}
