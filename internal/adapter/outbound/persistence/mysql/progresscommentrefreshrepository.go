package mysql

import (
	"context"
	"fmt"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/progresscomment"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *ReviewRunStore) EnsureProgressCommentRefresh(ctx context.Context, refresh progresscomment.Refresh) (bool, error) {
	if _, valid := reviewworkflow.ProgressMarkerKey(refresh.Marker); !valid {
		return false, fmt.Errorf("진행 코멘트 갱신 marker가 올바르지 않습니다")
	}
	if refresh.Target.InstallationID == 0 || refresh.Target.Owner == "" || refresh.Target.Repository == "" || refresh.Target.Number <= 0 {
		return false, fmt.Errorf("진행 코멘트 갱신 대상이 올바르지 않습니다")
	}
	if refresh.NextRefreshAt.IsZero() || !refresh.ExpiresAt.After(refresh.NextRefreshAt) {
		return false, fmt.Errorf("진행 코멘트 갱신 시간이 올바르지 않습니다")
	}
	entry := model.ProgressCommentOwnership{
		Marker:            refresh.Marker,
		InstallationID:    refresh.Target.InstallationID,
		Owner:             refresh.Target.Owner,
		Repository:        refresh.Target.Repository,
		Number:            refresh.Target.Number,
		RefreshSequence:   refresh.Sequence,
		CreateNotBefore:   timePointer(refresh.CreateNotBefore),
		NextRefreshAt:     timePointer(refresh.NextRefreshAt),
		RefreshExpiresAt:  timePointer(refresh.ExpiresAt),
		CleanupLeaseToken: "",
		CreatedAt:         refresh.CreatedAt,
		UpdatedAt:         refresh.CreatedAt,
	}
	active := false
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		created := transaction.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 1 {
			active = true
			return nil
		}
		var stored model.ProgressCommentOwnership
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("marker = ?", refresh.Marker).First(&stored).Error; err != nil {
			return err
		}
		if stored.InstallationID != 0 && (stored.InstallationID != refresh.Target.InstallationID || stored.Owner != refresh.Target.Owner || stored.Repository != refresh.Target.Repository || stored.Number != refresh.Target.Number) {
			return fmt.Errorf("진행 코멘트 갱신 대상이 기존 소유권과 다릅니다")
		}
		active = stored.NextRefreshAt != nil && stored.RefreshStopRequestedAt == nil
		if active {
			updates := map[string]any{
				"installation_id": refresh.Target.InstallationID,
				"owner":           refresh.Target.Owner,
				"repository":      refresh.Target.Repository,
				"number":          refresh.Target.Number,
				"updated_at":      refresh.CreatedAt,
			}
			if stored.RefreshSequence < refresh.Sequence {
				updates["refresh_sequence"] = refresh.Sequence
			}
			if stored.CreateNotBefore == nil || refresh.CreateNotBefore.After(*stored.CreateNotBefore) {
				updates["create_not_before"] = refresh.CreateNotBefore
			}
			if stored.RefreshExpiresAt == nil || refresh.ExpiresAt.After(*stored.RefreshExpiresAt) {
				updates["refresh_expires_at"] = refresh.ExpiresAt
			}
			if err := transaction.Model(&model.ProgressCommentOwnership{}).Where("marker = ?", refresh.Marker).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("진행 코멘트 갱신을 등록하지 못했습니다: %w", err)
	}
	return active, nil
}

func (r *ReviewRunStore) ClaimProgressCommentRefreshes(ctx context.Context, claimedAt time.Time, leaseExpiresAt time.Time, limit int) ([]progresscomment.Refresh, error) {
	if claimedAt.IsZero() || !leaseExpiresAt.After(claimedAt) {
		return nil, fmt.Errorf("진행 코멘트 갱신 claim 시간이 올바르지 않습니다")
	}
	if limit <= 0 {
		limit = 1
	}
	leaseDuration := leaseExpiresAt.Sub(claimedAt)
	entries := make([]model.ProgressCommentOwnership, 0, limit)
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := transaction.Where("review_run_id = 0 AND refresh_expires_at IS NOT NULL AND refresh_expires_at <= ?", claimedAt).
			Where("cleanup_lease_token = '' OR cleanup_lease_expires_at IS NULL OR cleanup_lease_expires_at <= ?", claimedAt).
			Delete(&model.ProgressCommentOwnership{}).Error; err != nil {
			return err
		}
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("next_refresh_at IS NOT NULL AND next_refresh_at <= ?", claimedAt).
			Where("refresh_stop_requested_at IS NULL").
			Where("refresh_expires_at IS NOT NULL AND refresh_expires_at > ?", claimedAt).
			Where("cleanup_lease_token = '' OR cleanup_lease_expires_at IS NULL OR cleanup_lease_expires_at <= ?", claimedAt).
			Order("next_refresh_at ASC").Limit(limit).Find(&entries).Error; err != nil {
			return err
		}
		for index := range entries {
			leaseToken, err := newLeaseToken()
			if err != nil {
				return err
			}
			leaseNow, err := databaseTime(transaction)
			if err != nil {
				return err
			}
			effectiveLeaseExpiresAt := leaseNow.Add(leaseDuration)
			updated := transaction.Model(&model.ProgressCommentOwnership{}).
				Where("marker = ? AND review_run_id = ?", entries[index].Marker, entries[index].ReviewRunID).
				Updates(map[string]any{
					"cleanup_lease_token":      leaseToken,
					"cleanup_lease_expires_at": effectiveLeaseExpiresAt,
					"updated_at":               leaseNow,
				})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return reviewworkflow.ErrProgressCommentLeased
			}
			entries[index].CleanupLeaseToken = leaseToken
			entries[index].CleanupLeaseExpiresAt = &effectiveLeaseExpiresAt
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("진행 코멘트 갱신 대상을 claim하지 못했습니다: %w", err)
	}
	refreshes := make([]progresscomment.Refresh, 0, len(entries))
	for _, entry := range entries {
		refreshes = append(refreshes, progressRefreshOf(entry))
	}
	return refreshes, nil
}

func (r *ReviewRunStore) CompleteProgressCommentRefresh(ctx context.Context, refresh progresscomment.Refresh, completedAt time.Time, nextRefreshAt time.Time) error {
	updates := map[string]any{
		"refresh_sequence":         gorm.Expr("refresh_sequence + 1"),
		"next_refresh_at":          gorm.Expr("CASE WHEN refresh_stop_requested_at IS NULL THEN ? ELSE NULL END", nullableTime(nextRefreshAt)),
		"cleanup_lease_token":      "",
		"cleanup_lease_expires_at": nil,
		"updated_at":               completedAt,
	}
	updated := r.database.WithContext(ctx).Model(&model.ProgressCommentOwnership{}).
		Where("marker = ? AND review_run_id = ? AND cleanup_lease_token = ?", refresh.Marker, refresh.RunID, refresh.LeaseToken).
		Updates(updates)
	if updated.Error != nil {
		return fmt.Errorf("진행 코멘트 갱신 완료를 저장하지 못했습니다: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return reviewworkflow.ErrProgressCommentLeased
	}
	return nil
}

func (r *ReviewRunStore) RetryProgressCommentRefresh(ctx context.Context, refresh progresscomment.Refresh, failedAt time.Time, nextAttemptAt time.Time, uncertainUntil time.Time) error {
	leaseToken := ""
	var leaseExpiresAt any
	if uncertainUntil.After(failedAt) {
		leaseToken = refresh.LeaseToken
		leaseExpiresAt = uncertainUntil
	}
	updated := r.database.WithContext(ctx).Model(&model.ProgressCommentOwnership{}).
		Where("marker = ? AND review_run_id = ? AND cleanup_lease_token = ?", refresh.Marker, refresh.RunID, refresh.LeaseToken).
		Updates(map[string]any{
			"next_refresh_at":          gorm.Expr("CASE WHEN refresh_stop_requested_at IS NULL THEN ? ELSE NULL END", nullableTime(nextAttemptAt)),
			"cleanup_lease_token":      leaseToken,
			"cleanup_lease_expires_at": leaseExpiresAt,
			"updated_at":               failedAt,
		})
	if updated.Error != nil {
		return fmt.Errorf("진행 코멘트 갱신 재시도를 저장하지 못했습니다: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return reviewworkflow.ErrProgressCommentLeased
	}
	return nil
}

func progressRefreshOf(entry model.ProgressCommentOwnership) progresscomment.Refresh {
	return progresscomment.Refresh{
		Marker: entry.Marker,
		RunID:  entry.ReviewRunID,
		Target: pullrequest.Target{
			InstallationID: entry.InstallationID,
			Owner:          entry.Owner,
			Repository:     entry.Repository,
			Number:         entry.Number,
		},
		Sequence:        entry.RefreshSequence,
		CreatedAt:       entry.CreatedAt,
		CreateNotBefore: timeValue(entry.CreateNotBefore),
		NextRefreshAt:   timeValue(entry.NextRefreshAt),
		ExpiresAt:       timeValue(entry.RefreshExpiresAt),
		LeaseToken:      entry.CleanupLeaseToken,
		LeaseExpiresAt:  timeValue(entry.CleanupLeaseExpiresAt),
	}
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func timeValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}
