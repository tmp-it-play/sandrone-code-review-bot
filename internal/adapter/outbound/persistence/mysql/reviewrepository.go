package mysql

import (
	"context"
	"errors"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/mapper"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ReviewRepository struct {
	database *gorm.DB
}

func NewReviewRepository(database *gorm.DB) *ReviewRepository {
	return &ReviewRepository{database: database}
}

func (r *ReviewRepository) Save(ctx context.Context, record review.Record) (uint64, error) {
	id := uint64(0)
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		storedID, created, err := saveReviewEntry(transaction, record)
		if err != nil {
			return err
		}
		id = storedID
		if created {
			return nil
		}
		var existing model.Review
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing, id).Error; err != nil {
			return err
		}
		if !shouldReplaceReview(review.Outcome(existing.Outcome), record.Outcome) {
			return nil
		}
		return transaction.Model(&model.Review{}).Where("id = ?", id).Updates(reviewUpdateValues(record)).Error
	})
	if err != nil {
		return 0, fmt.Errorf("리뷰 기록을 저장하지 못했습니다: %w", err)
	}
	return id, nil
}

func (r *ReviewRepository) SaveWithFindings(ctx context.Context, record review.Record, target pullrequest.Target, findings []review.Finding) (uint64, error) {
	reviewID := uint64(0)
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		id, created, err := saveReviewEntry(transaction, record)
		if err != nil {
			return err
		}
		reviewID = id
		writeFindings := created
		if !created {
			var existing model.Review
			if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing, id).Error; err != nil {
				return err
			}
			if shouldReplaceReview(review.Outcome(existing.Outcome), record.Outcome) {
				if err := transaction.Model(&model.Review{}).Where("id = ?", id).Updates(reviewUpdateValues(record)).Error; err != nil {
					return err
				}
				if err := transaction.Where("review_id = ?", id).Delete(&model.Finding{}).Error; err != nil {
					return err
				}
				writeFindings = true
			}
		}
		if !writeFindings || len(findings) == 0 {
			return nil
		}
		entries := make([]model.Finding, 0, len(findings))
		for _, finding := range findings {
			entries = append(entries, mapper.ToFindingModel(reviewID, target, finding))
		}
		return transaction.CreateInBatches(&entries, 50).Error
	})
	if err != nil {
		return 0, fmt.Errorf("리뷰 기록과 지적을 저장하지 못했습니다: %w", err)
	}
	return reviewID, nil
}

func (r *ReviewRepository) Recent(ctx context.Context, limit int) ([]review.Record, error) {
	if limit <= 0 {
		limit = 50
	}
	var entries []model.Review
	if err := r.database.WithContext(ctx).Order("id DESC").Limit(limit).Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("리뷰 목록을 읽지 못했습니다: %w", err)
	}
	records := make([]review.Record, 0, len(entries))
	for _, entry := range entries {
		records = append(records, mapper.ToReviewRecord(entry))
	}
	return records, nil
}

func (r *ReviewRepository) ByID(ctx context.Context, id uint64) (review.Record, error) {
	var entry model.Review
	if err := r.database.WithContext(ctx).First(&entry, id).Error; err != nil {
		return review.Record{}, fmt.Errorf("리뷰 기록을 찾지 못했습니다: %w", err)
	}
	return mapper.ToReviewRecord(entry), nil
}

func (r *ReviewRepository) ByRunID(ctx context.Context, runID uint64) (review.Record, bool, error) {
	var entry model.Review
	if err := r.database.WithContext(ctx).Where("review_run_id = ?", runID).First(&entry).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return review.Record{}, false, nil
		}
		return review.Record{}, false, fmt.Errorf("리뷰 실행의 기록을 찾지 못했습니다: %w", err)
	}
	return mapper.ToReviewRecord(entry), true, nil
}

func saveReviewEntry(database *gorm.DB, record review.Record) (uint64, bool, error) {
	entry := mapper.ToReviewModel(record)
	if record.RunID == 0 {
		if err := database.Create(&entry).Error; err != nil {
			return 0, false, err
		}
		return entry.ID, true, nil
	}
	if err := database.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "review_run_id"}},
		DoNothing: true,
	}).Create(&entry).Error; err != nil {
		return 0, false, err
	}
	entry = model.Review{}
	if err := database.Where("review_run_id = ?", record.RunID).First(&entry).Error; err != nil {
		return 0, false, err
	}
	return entry.ID, false, nil
}

func shouldReplaceReview(existing review.Outcome, incoming review.Outcome) bool {
	return existing == review.OutcomeUnavailable || existing == incoming
}

func reviewUpdateValues(record review.Record) map[string]any {
	return map[string]any{
		"owner":          record.Owner,
		"repository":     record.Repository,
		"number":         record.Number,
		"head_sha":       record.HeadSHA,
		"trigger":        string(record.Trigger),
		"outcome":        string(record.Outcome),
		"provider":       record.Provider,
		"model":          record.Model,
		"inline_count":   record.InlineCount,
		"fallback_count": record.FallbackCount,
		"detail":         record.Detail,
		"started_at":     record.StartedAt,
		"finished_at":    record.FinishedAt,
	}
}
