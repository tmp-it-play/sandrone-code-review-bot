package mysql

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"github.com/it-play/sandrone-code-review-bot/internal/core/textrevision"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *ReviewSummaryStore) ReviewTextRevision(ctx context.Context, runID uint64, lease string) (textrevision.Checkpoint, bool, error) {
	checkpoint := textrevision.Checkpoint{}
	found := false
	err := r.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireRunLease(tx, runID, lease); err != nil {
			return err
		}
		var entry model.ReviewSummary
		if err := tx.Where("review_run_id = ?", runID).First(&entry).Error; err != nil {
			return err
		}
		if entry.TextRevisionJSON == "" {
			return nil
		}
		var err error
		checkpoint, err = textrevision.DecodeCheckpoint(entry.TextRevisionJSON)
		found = err == nil
		return err
	})
	return checkpoint, found, err
}

func (r *ReviewSummaryStore) SaveReviewTextRevision(ctx context.Context, runID uint64, lease string, checkpoint textrevision.Checkpoint) error {
	encoded, err := checkpoint.Encode()
	if err != nil {
		return err
	}
	return r.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireRunLease(tx, runID, lease); err != nil {
			return err
		}
		var entry model.ReviewSummary
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("review_run_id = ?", runID).First(&entry).Error; err != nil {
			return err
		}
		if entry.TextRevisionJSON != "" {
			if entry.TextRevisionJSON != encoded {
				return reviewworkflow.ErrPlanChanged
			}
			return nil
		}
		return tx.Model(&entry).Update("text_revision_json", encoded).Error
	})
}
