package mysql

import (
	"strings"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func registerLatestRun(transaction *gorm.DB, run model.ReviewRun, registeredAt time.Time) (bool, bool, error) {
	var snapshotObservedAt *time.Time
	if !run.SnapshotObservedAt.IsZero() {
		snapshotObservedAt = &run.SnapshotObservedAt
	}
	state := model.PullRequestState{
		Owner:            run.Owner,
		Repository:       run.Repository,
		Number:           run.Number,
		LatestRunID:      run.ID,
		LatestHeadSHA:    run.HeadSHA,
		LatestObservedAt: snapshotObservedAt,
		LatestOrderKey:   run.SnapshotOrderKey,
		LatestRunAt:      &registeredAt,
		UpdatedAt:        registeredAt,
	}
	if err := transaction.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
		return false, false, err
	}
	state = model.PullRequestState{}
	if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("owner = ? AND repository = ? AND number = ?", run.Owner, run.Repository, run.Number).First(&state).Error; err != nil {
		return false, false, err
	}
	if latestRunIsNewer(state, run) {
		return false, false, nil
	}
	latestRunAt := registeredAt
	if state.LatestRunID == run.ID && state.LatestRunAt != nil {
		latestRunAt = *state.LatestRunAt
	}
	claimActive := state.PublishingRunID != 0 && state.PublishingLeaseExpiresAt != nil && state.PublishingLeaseExpiresAt.After(registeredAt)
	if claimActive {
		if state.LatestRunID != run.ID || state.LatestRunAt == nil {
			updated := transaction.Model(&model.PullRequestState{}).Where("id = ?", state.ID).Updates(map[string]any{
				"latest_run_id":      run.ID,
				"latest_head_sha":    run.HeadSHA,
				"latest_observed_at": snapshotObservedAt,
				"latest_order_key":   run.SnapshotOrderKey,
				"latest_run_at":      latestRunAt,
				"updated_at":         registeredAt,
			})
			if updated.Error != nil {
				return false, false, updated.Error
			}
			if updated.RowsAffected != 1 {
				return false, false, reviewworkflow.ErrPublicationLeased
			}
		}
		return false, true, nil
	}
	err := transaction.Model(&model.PullRequestState{}).Where("id = ?", state.ID).Updates(map[string]any{
		"latest_run_id":               run.ID,
		"latest_head_sha":             run.HeadSHA,
		"latest_observed_at":          snapshotObservedAt,
		"latest_order_key":            run.SnapshotOrderKey,
		"latest_run_at":               latestRunAt,
		"publishing_run_id":           0,
		"publishing_head_sha":         "",
		"publishing_lease_token":      "",
		"publishing_lease_expires_at": nil,
		"updated_at":                  registeredAt,
	}).Error
	return err == nil, false, err
}

func latestRunIsNewer(state model.PullRequestState, run model.ReviewRun) bool {
	if state.LatestObservedAt == nil {
		return run.SnapshotObservedAt.IsZero() && state.LatestRunID > run.ID
	}
	if run.SnapshotObservedAt.IsZero() {
		return true
	}
	if state.LatestObservedAt.After(run.SnapshotObservedAt) {
		return true
	}
	if state.LatestObservedAt.Before(run.SnapshotObservedAt) {
		return false
	}
	if state.LatestOrderKey != run.SnapshotOrderKey && latestOrderKeysAreComparable(state.LatestOrderKey, run.SnapshotOrderKey) {
		return state.LatestOrderKey > run.SnapshotOrderKey
	}
	return state.LatestRunID > run.ID
}

func latestOrderKeysAreComparable(left string, right string) bool {
	leftScope, _, leftFound := strings.Cut(left, ":")
	rightScope, _, rightFound := strings.Cut(right, ":")
	if !leftFound || !rightFound || leftScope != rightScope {
		return false
	}
	return leftScope == "issue" || leftScope == "review"
}
