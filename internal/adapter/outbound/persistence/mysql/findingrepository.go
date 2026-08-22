package mysql

import (
	"context"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/mapper"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"gorm.io/gorm"
)

type FindingRepository struct {
	database *gorm.DB
}

func NewFindingRepository(database *gorm.DB) *FindingRepository {
	return &FindingRepository{database: database}
}

func (r *FindingRepository) SaveAll(ctx context.Context, reviewID uint64, target pullrequest.Target, findings []review.Finding) error {
	if len(findings) == 0 {
		return nil
	}
	entries := make([]model.Finding, 0, len(findings))
	for _, finding := range findings {
		entries = append(entries, mapper.ToFindingModel(reviewID, target, finding))
	}
	if err := r.database.WithContext(ctx).CreateInBatches(&entries, 50).Error; err != nil {
		return fmt.Errorf("지적을 저장하지 못했다: %w", err)
	}
	return nil
}

func (r *FindingRepository) Fingerprints(ctx context.Context, target pullrequest.Target) (map[string]struct{}, error) {
	var fingerprints []string
	err := r.database.WithContext(ctx).
		Model(&model.Finding{}).
		Where("owner = ? AND repository = ? AND number = ?", target.Owner, target.Repository, target.Number).
		Pluck("fingerprint", &fingerprints).Error
	if err != nil {
		return nil, fmt.Errorf("기존 지적을 읽지 못했다: %w", err)
	}
	known := make(map[string]struct{}, len(fingerprints))
	for _, fingerprint := range fingerprints {
		known[fingerprint] = struct{}{}
	}
	return known, nil
}

func (r *FindingRepository) ByReview(ctx context.Context, reviewID uint64) ([]review.Finding, error) {
	var entries []model.Finding
	if err := r.database.WithContext(ctx).Where("review_id = ?", reviewID).Order("id ASC").Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("리뷰의 지적을 읽지 못했다: %w", err)
	}
	findings := make([]review.Finding, 0, len(entries))
	for _, entry := range entries {
		findings = append(findings, mapper.ToFinding(entry))
	}
	return findings, nil
}
