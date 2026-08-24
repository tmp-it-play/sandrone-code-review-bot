package mysql

import (
	"context"
	"fmt"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/mapper"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
)

type ReviewPublicationCandidateStore struct {
	database *gorm.DB
}

func NewReviewPublicationCandidateStore(database *gorm.DB) *ReviewPublicationCandidateStore {
	return &ReviewPublicationCandidateStore{database: database}
}

func (s *ReviewPublicationCandidateStore) PublicationCandidateHighWatermark(ctx context.Context, before time.Time) (uint64, error) {
	var highWatermark uint64
	row := s.database.WithContext(ctx).Model(&model.ReviewRun{}).
		Where("status = ? AND heartbeat_at <= ?", string(reviewworkflow.RunStatusPublishing), before).
		Select("COALESCE(MAX(id), 0)").Row()
	if err := row.Scan(&highWatermark); err != nil {
		return 0, fmt.Errorf("게시 조정 대상 high watermark를 읽지 못했습니다: %w", err)
	}
	return highWatermark, nil
}

func (s *ReviewPublicationCandidateStore) PublicationCandidates(ctx context.Context, before time.Time, afterID uint64, throughID uint64, limit int) ([]reviewworkflow.Run, error) {
	if limit <= 0 {
		limit = 100
	}
	var entries []model.ReviewRun
	if err := s.database.WithContext(ctx).
		Where("id > ? AND id <= ? AND status = ? AND heartbeat_at <= ? AND (lease_expires_at IS NULL OR lease_expires_at <= ?)", afterID, throughID, string(reviewworkflow.RunStatusPublishing), before, before).
		Order("id ASC").Limit(limit).Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("게시 조정 대상 리뷰 실행을 읽지 못했습니다: %w", err)
	}
	runs := make([]reviewworkflow.Run, 0, len(entries))
	for _, entry := range entries {
		runs = append(runs, mapper.ToReviewRun(entry))
	}
	return runs, nil
}
