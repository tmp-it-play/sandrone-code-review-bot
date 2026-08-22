package mysql

import (
	"context"
	"errors"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PullRequestStateRepository struct {
	database *gorm.DB
}

func NewPullRequestStateRepository(database *gorm.DB) *PullRequestStateRepository {
	return &PullRequestStateRepository{database: database}
}

func (r *PullRequestStateRepository) LastReviewedSHA(ctx context.Context, target pullrequest.Target) (string, error) {
	var entry model.PullRequestState
	err := r.database.WithContext(ctx).
		Where("owner = ? AND repository = ? AND number = ?", target.Owner, target.Repository, target.Number).
		First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("마지막 리뷰 커밋을 읽지 못했다: %w", err)
	}
	return entry.LastReviewedSHA, nil
}

func (r *PullRequestStateRepository) SetLastReviewedSHA(ctx context.Context, target pullrequest.Target, sha string) error {
	entry := model.PullRequestState{
		Owner:           target.Owner,
		Repository:      target.Repository,
		Number:          target.Number,
		LastReviewedSHA: sha,
	}
	err := r.database.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "owner"}, {Name: "repository"}, {Name: "number"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_reviewed_sha", "updated_at"}),
	}).Create(&entry).Error
	if err != nil {
		return fmt.Errorf("마지막 리뷰 커밋을 저장하지 못했다: %w", err)
	}
	return nil
}
