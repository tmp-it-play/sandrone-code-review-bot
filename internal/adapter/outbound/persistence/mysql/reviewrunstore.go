package mysql

import (
	"context"
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
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		candidate := mapper.ToReviewRunModel(run)
		if err := transaction.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
			return err
		}
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_key = ?", run.Key).First(&entry).Error; err != nil {
			return err
		}
		currentAt, err := databaseTime(transaction)
		if err != nil {
			return err
		}
		if !reviewworkflow.RunStatus(entry.Status).IsTerminal() || entry.ExpiresAt == nil || entry.ExpiresAt.After(currentAt) {
			return nil
		}
		if err := deleteExpiredRunLifecycle(transaction, entry.ID); err != nil {
			return err
		}
		entry = mapper.ToReviewRunModel(run)
		return transaction.Create(&entry).Error
	})
	if err != nil {
		return reviewworkflow.Run{}, fmt.Errorf("리뷰 실행을 생성하거나 읽지 못했습니다: %w", err)
	}
	return mapper.ToReviewRun(entry), nil
}
