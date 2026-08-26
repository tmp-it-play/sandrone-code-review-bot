package mysql

import (
	"context"
	"errors"
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
	storedProgressMarkers := make(map[uint64]string)
	progressMarkers := make(map[uint64]string)
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
		if len(entries) == 0 {
			return nil
		}
		runIDs := make([]uint64, 0, len(entries))
		for _, entry := range entries {
			runIDs = append(runIDs, entry.ReviewRunID)
		}
		var runs []struct {
			ID             uint64
			ProgressMarker string
		}
		if err := transaction.Model(&model.ReviewRun{}).Select("id", "progress_marker").Where("id IN ?", runIDs).Find(&runs).Error; err != nil {
			return err
		}
		entriesByRunID := make(map[uint64]model.PublicationInvalidation, len(entries))
		for _, entry := range entries {
			entriesByRunID[entry.ReviewRunID] = entry
		}
		for _, run := range runs {
			storedProgressMarkers[run.ID] = run.ProgressMarker
			entry := entriesByRunID[run.ID]
			owned, err := claimProgressCommentCleanup(transaction, run.ProgressMarker, run.ID, entry.LeaseToken, claimedAt, leaseExpiresAt)
			if err != nil {
				return err
			}
			if owned {
				progressMarkers[run.ID] = run.ProgressMarker
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("게시 무효화 대상을 claim하지 못했습니다: %w", err)
	}
	invalidations := make([]reviewworkflow.PublicationInvalidation, 0, len(entries))
	for _, entry := range entries {
		invalidation := toPublicationInvalidation(entry)
		invalidation.ProgressMarker = storedProgressMarkers[entry.ReviewRunID]
		invalidation.ProgressOwned = true
		if storedMarker := invalidation.ProgressMarker; storedMarker != "" {
			invalidation.ProgressOwned = progressMarkers[entry.ReviewRunID] != ""
			if invalidation.Marker == storedMarker && !invalidation.ProgressOwned {
				invalidation.Marker = ""
			}
		}
		invalidations = append(invalidations, invalidation)
	}
	return invalidations, nil
}

func (s *PublicationInvalidationStore) CompletePublicationInvalidation(ctx context.Context, invalidationID uint64, leaseToken string, resolvedAt time.Time) error {
	err := s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		updated := transaction.Model(&model.PublicationInvalidation{}).
			Where("id = ? AND lease_token = ? AND resolved_at IS NULL", invalidationID, leaseToken).
			Updates(map[string]any{
				"lease_token":      "",
				"lease_expires_at": nil,
				"resolved_at":      resolvedAt,
				"last_error":       "",
				"updated_at":       resolvedAt,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return publication.ErrLeaseLost
		}
		return releaseProgressCommentCleanup(transaction, leaseToken, resolvedAt)
	})
	if err != nil {
		return fmt.Errorf("게시 무효화 완료를 저장하지 못했습니다: %w", err)
	}
	return nil
}

func (s *PublicationInvalidationStore) RetryPublicationInvalidation(ctx context.Context, invalidationID uint64, leaseToken string, failedAt time.Time, nextAttemptAt time.Time, uncertainUntil time.Time, failure string) error {
	err := s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		updated := transaction.Model(&model.PublicationInvalidation{}).
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
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return publication.ErrLeaseLost
		}
		if uncertainUntil.After(failedAt) {
			return transaction.Model(&model.ProgressCommentOwnership{}).
				Where("cleanup_lease_token = ?", leaseToken).
				Updates(map[string]any{
					"cleanup_lease_expires_at": uncertainUntil,
					"updated_at":               failedAt,
				}).Error
		}
		return releaseProgressCommentCleanup(transaction, leaseToken, failedAt)
	})
	if err != nil {
		return fmt.Errorf("게시 무효화 재시도를 저장하지 못했습니다: %w", err)
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
	var stored model.PublicationInvalidation
	storedErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("review_run_id = ?", run.ID).First(&stored).Error
	if storedErr != nil && !errors.Is(storedErr, gorm.ErrRecordNotFound) {
		return storedErr
	}
	if run.ProgressMarker != "" && value.Marker == run.ProgressMarker {
		owned, err := ownsProgressCommentForUpdate(transaction, run.ID, run.ProgressMarker)
		if err != nil {
			return err
		}
		if !owned {
			return nil
		}
	}
	if errors.Is(storedErr, gorm.ErrRecordNotFound) {
		if err := transaction.Create(&entry).Error; err != nil {
			return err
		}
		stored = entry
	}
	if stored.ID == 0 {
		if err := transaction.Where("review_run_id = ?", run.ID).First(&stored).Error; err != nil {
			return err
		}
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
