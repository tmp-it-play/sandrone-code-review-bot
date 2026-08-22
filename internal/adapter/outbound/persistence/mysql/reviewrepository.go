package mysql

import (
	"context"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/mapper"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"gorm.io/gorm"
)

type ReviewRepository struct {
	database *gorm.DB
}

func NewReviewRepository(database *gorm.DB) *ReviewRepository {
	return &ReviewRepository{database: database}
}

func (r *ReviewRepository) Save(ctx context.Context, record review.Record) (uint64, error) {
	entry := mapper.ToReviewModel(record)
	if err := r.database.WithContext(ctx).Create(&entry).Error; err != nil {
		return 0, fmt.Errorf("리뷰 기록을 저장하지 못했습니다: %w", err)
	}
	return entry.ID, nil
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
