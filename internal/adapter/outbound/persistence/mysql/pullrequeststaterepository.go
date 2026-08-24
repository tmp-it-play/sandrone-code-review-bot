package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

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

func (r *PullRequestStateRepository) LastReviewedSHA(ctx context.Context, target pullrequest.Target, activeAfter time.Time) (string, error) {
	var entry model.PullRequestState
	err := r.database.WithContext(ctx).
		Model(&model.PullRequestState{}).
		Select("pull_request_states.*").
		Joins("LEFT JOIN review_runs ON review_runs.id = pull_request_states.last_reviewed_run_id").
		Where("pull_request_states.owner = ? AND pull_request_states.repository = ? AND pull_request_states.number = ? AND pull_request_states.last_reviewed_at IS NOT NULL", target.Owner, target.Repository, target.Number).
		Where("(pull_request_states.last_reviewed_run_id <> 0 AND review_runs.expires_at > CURRENT_TIMESTAMP(6)) OR (pull_request_states.last_reviewed_run_id = 0 AND pull_request_states.last_reviewed_at > ?)", activeAfter).
		First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("마지막 리뷰 커밋을 읽지 못했습니다: %w", err)
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
		return fmt.Errorf("마지막 리뷰 커밋을 저장하지 못했습니다: %w", err)
	}
	return nil
}
