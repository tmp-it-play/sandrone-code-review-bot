package mysql

import (
	"context"
	"errors"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/mapper"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ReviewRunStore struct {
	database *gorm.DB
}

func NewReviewRunStore(database *gorm.DB) *ReviewRunStore {
	return &ReviewRunStore{database: database}
}

func (r *ReviewRunStore) CreateOrGetRun(ctx context.Context, run reviewworkflow.Run) (reviewworkflow.Run, error) {
	entry := model.ReviewRun{}
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		entry = model.ReviewRun{}
		err = r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
			var createErr error
			entry, createErr = createOrGetReviewRun(transaction, run)
			return createErr
		})
		if !errors.Is(err, errProgressCommentOwnershipRace) {
			break
		}
	}
	if err != nil {
		return reviewworkflow.Run{}, fmt.Errorf("리뷰 실행을 생성하거나 읽지 못했습니다: %w", err)
	}
	return mapper.ToReviewRun(entry), nil
}

func createOrGetReviewRun(transaction *gorm.DB, run reviewworkflow.Run) (model.ReviewRun, error) {
	candidate := mapper.ToReviewRunModel(run)
	if err := transaction.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
		return model.ReviewRun{}, err
	}
	entry := model.ReviewRun{}
	if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_key = ?", run.Key).First(&entry).Error; err != nil {
		return model.ReviewRun{}, err
	}
	if entry.ProgressMarker != "" && candidate.ProgressMarker != "" && entry.ProgressMarker != candidate.ProgressMarker {
		return model.ReviewRun{}, fmt.Errorf("리뷰 실행의 진행 marker가 기존 값과 다릅니다")
	}
	if entry.ProgressMarker == "" && candidate.ProgressMarker != "" {
		if err := transaction.Model(&model.ReviewRun{}).Where("id = ? AND progress_marker = ''", entry.ID).Update("progress_marker", candidate.ProgressMarker).Error; err != nil {
			return model.ReviewRun{}, err
		}
		entry.ProgressMarker = candidate.ProgressMarker
	}
	currentAt, err := databaseTime(transaction)
	if err != nil {
		return model.ReviewRun{}, err
	}
	if !reviewworkflow.RunStatus(entry.Status).IsTerminal() || entry.ExpiresAt == nil || entry.ExpiresAt.After(currentAt) {
		if err := ensureProgressCommentOwnership(transaction, entry, currentAt); err != nil {
			return model.ReviewRun{}, err
		}
		return entry, nil
	}
	if err := deleteExpiredRunLifecycle(transaction, entry.ID); err != nil {
		return model.ReviewRun{}, err
	}
	entry = mapper.ToReviewRunModel(run)
	if err := transaction.Create(&entry).Error; err != nil {
		return model.ReviewRun{}, err
	}
	if err := ensureProgressCommentOwnership(transaction, entry, currentAt); err != nil {
		return model.ReviewRun{}, err
	}
	return entry, nil
}
