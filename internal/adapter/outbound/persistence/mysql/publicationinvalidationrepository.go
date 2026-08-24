package mysql

import (
	"context"
	"fmt"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *PublicationInvalidationStore) ClaimPublicationInvalidations(ctx context.Context, claimedAt time.Time, leaseExpiresAt time.Time, limit int) ([]reviewworkflow.PublicationInvalidation, error) {
	if claimedAt.IsZero() || !leaseExpiresAt.After(claimedAt) {
		return nil, fmt.Errorf("게시 무효화 claim 시간이 올바르지 않습니다")
	}
	if limit <= 0 {
		limit = 100
	}
	entries := make([]model.PublicationInvalidation, 0, limit)
	err := s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("resolved_at IS NULL AND expires_at > ? AND next_attempt_at <= ?", claimedAt, claimedAt).
			Where("lease_token = '' OR lease_expires_at IS NULL OR lease_expires_at <= ?", claimedAt).
			Order("id ASC").Limit(limit).Find(&entries).Error; err != nil {
			return err
		}
		for index := range entries {
			leaseToken, err := newLeaseToken()
			if err != nil {
				return err
			}
			updated := transaction.Model(&model.PublicationInvalidation{}).
				Where("id = ? AND resolved_at IS NULL AND expires_at > ?", entries[index].ID, claimedAt).
				Updates(map[string]any{
					"lease_token":      leaseToken,
					"lease_expires_at": leaseExpiresAt,
					"updated_at":       claimedAt,
				})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return publication.ErrLeaseLost
			}
			entries[index].LeaseToken = leaseToken
			entries[index].LeaseExpiresAt = &leaseExpiresAt
			entries[index].UpdatedAt = claimedAt
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("게시 무효화 대상을 claim하지 못했습니다: %w", err)
	}
	invalidations := make([]reviewworkflow.PublicationInvalidation, 0, len(entries))
	for _, entry := range entries {
		invalidations = append(invalidations, toPublicationInvalidation(entry))
	}
	return invalidations, nil
}

func (s *PublicationInvalidationStore) CompletePublicationInvalidation(ctx context.Context, invalidationID uint64, leaseToken string, resolvedAt time.Time) error {
	updated := s.database.WithContext(ctx).Model(&model.PublicationInvalidation{}).
		Where("id = ? AND lease_token = ? AND resolved_at IS NULL", invalidationID, leaseToken).
		Updates(map[string]any{
			"lease_token":      "",
			"lease_expires_at": nil,
			"resolved_at":      resolvedAt,
			"last_error":       "",
			"updated_at":       resolvedAt,
		})
	if updated.Error != nil {
		return fmt.Errorf("게시 무효화 완료를 저장하지 못했습니다: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return publication.ErrLeaseLost
	}
	return nil
}

func (s *PublicationInvalidationStore) RetryPublicationInvalidation(ctx context.Context, invalidationID uint64, leaseToken string, failedAt time.Time, nextAttemptAt time.Time, failure string) error {
	updated := s.database.WithContext(ctx).Model(&model.PublicationInvalidation{}).
		Where("id = ? AND lease_token = ? AND resolved_at IS NULL", invalidationID, leaseToken).
		Updates(map[string]any{
			"attempts":         gorm.Expr("attempts + 1"),
			"last_error":       boundedText(failure, 1000),
			"next_attempt_at":  nextAttemptAt,
			"lease_token":      "",
			"lease_expires_at": nil,
			"updated_at":       failedAt,
		})
	if updated.Error != nil {
		return fmt.Errorf("게시 무효화 재시도를 저장하지 못했습니다: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return publication.ErrLeaseLost
	}
	return nil
}

func persistPublicationInvalidation(transaction *gorm.DB, run model.ReviewRun, value reviewworkflow.PublicationInvalidation, terminalAt time.Time, expiresAt time.Time) error {
	if value.Marker == "" || value.Reason == "" || terminalAt.IsZero() || !expiresAt.After(terminalAt) {
		return fmt.Errorf("게시 무효화 정보가 올바르지 않습니다")
	}
	nextAttemptAt := value.NextAttemptAt
	if nextAttemptAt.IsZero() || nextAttemptAt.Before(terminalAt) {
		nextAttemptAt = terminalAt
	}
	entry := model.PublicationInvalidation{
		ReviewRunID:    run.ID,
		InstallationID: run.InstallationID,
		Owner:          run.Owner,
		Repository:     run.Repository,
		Number:         run.Number,
		Marker:         boundedText(value.Marker, 200),
		Reason:         boundedText(value.Reason, 2000),
		LastError:      boundedText(value.LastError, 1000),
		NextAttemptAt:  nextAttemptAt,
		ExpiresAt:      expiresAt,
		CreatedAt:      terminalAt,
		UpdatedAt:      terminalAt,
	}
	if err := transaction.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry).Error; err != nil {
		return err
	}
	var stored model.PublicationInvalidation
	if err := transaction.Where("review_run_id = ?", run.ID).First(&stored).Error; err != nil {
		return err
	}
	expiryDelta := stored.ExpiresAt.Sub(expiresAt)
	if stored.InstallationID != run.InstallationID || stored.Owner != run.Owner || stored.Repository != run.Repository || stored.Number != run.Number || stored.Marker != entry.Marker || stored.Reason != entry.Reason || expiryDelta < -time.Second || expiryDelta > time.Second {
		return fmt.Errorf("게시 무효화 정보가 기존 run과 충돌했습니다")
	}
	return nil
}

func toPublicationInvalidation(entry model.PublicationInvalidation) reviewworkflow.PublicationInvalidation {
	return reviewworkflow.PublicationInvalidation{
		ID:             entry.ID,
		RunID:          entry.ReviewRunID,
		InstallationID: entry.InstallationID,
		Owner:          entry.Owner,
		Repository:     entry.Repository,
		Number:         entry.Number,
		Marker:         entry.Marker,
		Reason:         entry.Reason,
		Attempts:       entry.Attempts,
		LastError:      entry.LastError,
		NextAttemptAt:  entry.NextAttemptAt,
		LeaseToken:     entry.LeaseToken,
		LeaseExpiresAt: entry.LeaseExpiresAt,
		ResolvedAt:     entry.ResolvedAt,
		ExpiresAt:      entry.ExpiresAt,
		CreatedAt:      entry.CreatedAt,
		UpdatedAt:      entry.UpdatedAt,
	}
}
