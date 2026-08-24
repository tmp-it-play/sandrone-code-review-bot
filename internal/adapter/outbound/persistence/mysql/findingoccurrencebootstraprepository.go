package mysql

import (
	"context"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const findingOccurrenceBootstrapCheckpoint = "finding-occurrences-v2"

type FindingOccurrenceBootstrapRepository struct {
	database *gorm.DB
}

func NewFindingOccurrenceBootstrapRepository(database *gorm.DB) *FindingOccurrenceBootstrapRepository {
	return &FindingOccurrenceBootstrapRepository{database: database}
}

func (r *FindingOccurrenceBootstrapRepository) BackfillBatch(ctx context.Context) (bool, error) {
	completed := false
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		now, err := databaseTime(transaction)
		if err != nil {
			return fmt.Errorf("발생 이력 migration 기준 시각을 읽지 못했습니다: %w", err)
		}
		checkpoint := model.MigrationCheckpoint{Name: findingOccurrenceBootstrapCheckpoint, UpdatedAt: now}
		if err := transaction.Clauses(clause.OnConflict{DoNothing: true}).Create(&checkpoint).Error; err != nil {
			return err
		}
		checkpoint = model.MigrationCheckpoint{}
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("name = ?", findingOccurrenceBootstrapCheckpoint).First(&checkpoint).Error; err != nil {
			return err
		}
		lastFindingID, found, err := backfillFindingOccurrencesBatch(transaction, checkpoint.Cursor, now)
		if err != nil {
			return err
		}
		if !found {
			if checkpoint.CompletedAt != nil {
				completed = true
				return nil
			}
			updated := transaction.Model(&model.MigrationCheckpoint{}).
				Where(map[string]any{"name": findingOccurrenceBootstrapCheckpoint, "cursor": checkpoint.Cursor}).
				Where("completed_at IS NULL").
				Updates(map[string]any{"completed_at": now, "updated_at": now})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return fmt.Errorf("발생 이력 migration 완료 checkpoint를 갱신하지 못했습니다")
			}
			completed = true
			return nil
		}
		updated := transaction.Model(&model.MigrationCheckpoint{}).
			Where(map[string]any{"name": findingOccurrenceBootstrapCheckpoint, "cursor": checkpoint.Cursor}).
			Updates(map[string]any{"cursor": lastFindingID, "completed_at": nil, "updated_at": now})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("발생 이력 migration cursor를 갱신하지 못했습니다")
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("기존 지적의 발생 이력 migration batch를 처리하지 못했습니다: %w", err)
	}
	return completed, nil
}
