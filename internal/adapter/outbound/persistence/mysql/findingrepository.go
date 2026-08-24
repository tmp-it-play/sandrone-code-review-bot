package mysql

import (
	"context"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/mapper"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"gorm.io/gorm"
)

type FindingRepository struct {
	database *gorm.DB
}

func NewFindingRepository(database *gorm.DB) *FindingRepository {
	return &FindingRepository{database: database}
}

func (r *FindingRepository) ByReview(ctx context.Context, reviewID uint64) ([]review.Finding, error) {
	var entries []model.Finding
	if err := r.database.WithContext(ctx).Where("review_id = ?", reviewID).Order("id ASC").Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("리뷰의 지적을 읽지 못했습니다: %w", err)
	}
	findings := make([]review.Finding, 0, len(entries))
	for _, entry := range entries {
		findings = append(findings, mapper.ToFinding(entry))
	}
	return findings, nil
}
