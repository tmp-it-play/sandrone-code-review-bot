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

func (r *ReviewPublicationStore) mutatePreparedPublication(ctx context.Context, runID uint64, token string, mutate func(*gorm.DB, model.ReviewPublication, time.Time) error) error {
	return r.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireReviewPublicationLease(tx, runID, token); err != nil {
			return err
		}
		var entry model.ReviewPublication
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("review_run_id = ?", runID).First(&entry).Error; err != nil {
			return err
		}
		now, err := databaseTime(tx)
		if err != nil {
			return err
		}
		if !entry.ExpiresAt.After(now) {
			return reviewworkflow.ErrPublicationExpired
		}
		if entry.Status != reviewworkflow.ReviewPublicationStatusPrepared {
			return reviewworkflow.ErrPublicationIncomplete
		}
		return mutate(tx.Model(&entry), entry, now)
	})
}

func (r *ReviewPublicationStore) BeginPublicationAttempt(ctx context.Context, runID uint64, token string) (reviewworkflow.PublicationAttempt, error) {
	var attempt reviewworkflow.PublicationAttempt
	err := r.mutatePreparedPublication(ctx, runID, token, func(row *gorm.DB, entry model.ReviewPublication, now time.Time) error {
		if entry.NextAttemptAt != nil && entry.NextAttemptAt.After(now) {
			return &publication.RetryAtError{At: *entry.NextAttemptAt, Cause: errors.New("리뷰 게시 재시도 대기 중")}
		}
		if entry.Attempts >= reviewworkflow.MaxPublicationAttempts {
			return fmt.Errorf("%w (%d): %s", reviewworkflow.ErrPublicationAttemptsExhausted, entry.Attempts, entry.LastFailure)
		}
		attempt = reviewworkflow.PublicationAttempt{Number: entry.Attempts + 1, Channel: entry.Channel}
		next := reviewworkflow.NextPublicationAttempt(now, attempt.Number, time.Time{})
		return row.Updates(map[string]any{"attempts": attempt.Number, "next_attempt_at": optionalRetryTime(next)}).Error
	})
	return attempt, err
}

func (r *ReviewPublicationStore) RecordPublicationFailure(ctx context.Context, runID uint64, token string, attempt int, retryAfter time.Time, detail string) (time.Time, error) {
	var next time.Time
	err := r.mutatePreparedPublication(ctx, runID, token, func(row *gorm.DB, entry model.ReviewPublication, now time.Time) error {
		if entry.Attempts != attempt {
			return reviewworkflow.ErrPublicationLeased
		}
		next = reviewworkflow.NextPublicationAttempt(now, attempt, retryAfter)
		return row.Updates(map[string]any{"next_attempt_at": optionalRetryTime(next), "last_failure": boundedText(detail, 1000)}).Error
	})
	return next, err
}

func (r *ReviewPublicationStore) SelectPublicationFallback(ctx context.Context, runID uint64, token string) error {
	return r.mutatePreparedPublication(ctx, runID, token, func(row *gorm.DB, _ model.ReviewPublication, _ time.Time) error {
		return row.Update("channel", reviewworkflow.ReviewPublicationChannelComment).Error
	})
}

func (r *ReviewPublicationStore) PublicationAttemptsExhausted(ctx context.Context, runID uint64) (bool, error) {
	var count int64
	err := r.database.WithContext(ctx).Model(&model.ReviewPublication{}).Where("review_run_id = ? AND status = ? AND attempts >= ?", runID, reviewworkflow.ReviewPublicationStatusPrepared, reviewworkflow.MaxPublicationAttempts).Count(&count).Error
	return count > 0, err
}

func optionalRetryTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}
