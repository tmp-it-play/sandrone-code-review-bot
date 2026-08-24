package mysql

import (
	"strings"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func updateReviewProjection(transaction *gorm.DB, run model.ReviewRun, outcome review.Outcome, detail string, finishedAt time.Time) error {
	runID := run.ID
	entry := model.Review{
		ReviewRunID: &runID,
		Owner:       run.Owner,
		Repository:  run.Repository,
		Number:      run.Number,
		HeadSHA:     run.HeadSHA,
		Trigger:     run.Trigger,
		Outcome:     string(outcome),
		Detail:      strings.TrimSpace(detail),
		StartedAt:   run.StartedAt,
		FinishedAt:  finishedAt,
	}
	return transaction.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "review_run_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"outcome":     entry.Outcome,
			"detail":      entry.Detail,
			"finished_at": entry.FinishedAt,
		}),
	}).Create(&entry).Error
}
