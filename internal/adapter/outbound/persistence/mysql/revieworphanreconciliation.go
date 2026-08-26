package mysql

import (
	"context"
	"fmt"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/mapper"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *ReviewRetentionRepository) ReconcileOrphans(ctx context.Context, staleBefore time.Time, terminalAt time.Time, expiresAt time.Time, limit int) (reviewworkflow.OrphanReconciliation, error) {
	if limit <= 0 {
		limit = 500
	}
	var runIDs []uint64
	err := r.database.WithContext(ctx).Model(&model.ReviewRun{}).
		Where("status NOT IN ? AND heartbeat_at <= ? AND (lease_expires_at IS NULL OR lease_expires_at <= ?)", orphanExcludedRunStatuses(), staleBefore, terminalAt).
		Order("id ASC").Limit(limit).Pluck("id", &runIDs).Error
	if err != nil {
		return reviewworkflow.OrphanReconciliation{}, fmt.Errorf("중단된 리뷰 실행을 찾지 못했습니다: %w", err)
	}
	reconciled := int64(0)
	for _, runID := range runIDs {
		err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
			var run model.ReviewRun
			if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, runID).Error; err != nil {
				return err
			}
			if reviewworkflow.RunStatus(run.Status).IsTerminal() || run.HeartbeatAt.After(staleBefore) || (run.LeaseExpiresAt != nil && run.LeaseExpiresAt.After(terminalAt)) {
				return nil
			}
			if err := transaction.Model(&model.ReviewUnit{}).
				Where("review_run_id = ? AND status IN ?", runID, []string{string(reviewworkflow.UnitStatusPending), string(reviewworkflow.UnitStatusRunning)}).
				Updates(map[string]any{
					"status":           string(reviewworkflow.UnitStatusFailed),
					"retryable":        false,
					"retry_at":         nil,
					"error_summary":    "7일 동안 진행되지 않아 종료됨",
					"finished_at":      terminalAt,
					"heartbeat_at":     terminalAt,
					"lease_token":      "",
					"lease_expires_at": nil,
				}).Error; err != nil {
				return err
			}
			if err := transaction.Model(&model.ReviewUnit{}).
				Where("review_run_id = ? AND retryable = ?", runID, true).
				Updates(map[string]any{"retryable": false, "retry_at": nil}).Error; err != nil {
				return err
			}
			if err := transaction.Model(&model.CoverageItem{}).
				Where("review_run_id = ? AND status IN ?", runID, []string{string(reviewworkflow.CoverageStatusIndexed), string(reviewworkflow.CoverageStatusPlanned)}).
				Update("status", string(reviewworkflow.CoverageStatusFailed)).Error; err != nil {
				return err
			}
			var coverageEntries []model.CoverageItem
			if err := transaction.Where("review_run_id = ?", runID).Find(&coverageEntries).Error; err != nil {
				return err
			}
			coverage := make([]reviewworkflow.CoverageItem, 0, len(coverageEntries))
			for _, entry := range coverageEntries {
				coverage = append(coverage, mapper.ToCoverageItem(entry))
			}
			summary := reviewworkflow.SummarizeCoverage(coverage)
			updated := transaction.Model(&model.ReviewRun{}).
				Where("id = ? AND status NOT IN ? AND heartbeat_at <= ? AND (lease_expires_at IS NULL OR lease_expires_at <= ?)", runID, orphanExcludedRunStatuses(), staleBefore, terminalAt).
				Updates(map[string]any{
					"status":            string(reviewworkflow.RunStatusFailed),
					"total_coverage":    summary.Total,
					"reviewed_coverage": summary.Reviewed,
					"failed_coverage":   summary.Failed,
					"deferred_coverage": summary.Deferred + summary.Pending,
					"skipped_coverage":  summary.Skipped,
					"error_summary":     "7일 동안 진행되지 않아 종료됨",
					"heartbeat_at":      terminalAt,
					"terminal_at":       terminalAt,
					"expires_at":        expiresAt,
					"lease_token":       "",
					"lease_expires_at":  nil,
				})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected == 1 {
				if err := transaction.Model(&model.PullRequestState{}).
					Where("owner = ? AND repository = ? AND number = ? AND publishing_run_id = ?", run.Owner, run.Repository, run.Number, run.ID).
					UpdateColumns(publicationClaimClearValues()).Error; err != nil {
					return err
				}
				progressMarker := run.ProgressMarker
				if progressMarker == "" {
					progressMarker = reviewworkflow.ProgressMarker(run.RunKey)
				}
				finalization := reviewworkflow.NewTerminalProgressFinalization(progressMarker)
				if finalization != nil {
					if invalidation := finalization.Invalidation(reviewworkflow.RunStatusFailed, terminalAt, expiresAt); invalidation != nil {
						if err := persistPublicationInvalidation(transaction, run, *invalidation, terminalAt, expiresAt); err != nil {
							return err
						}
					}
				}
				if err := stopProgressCommentRefreshForUpdate(transaction, run, terminalAt); err != nil {
					return err
				}
				if err := updateReviewProjection(transaction, run, review.OutcomeFailed, "7일 동안 게시 결과를 확인하지 못해 종료됨", terminalAt); err != nil {
					return err
				}
			}
			reconciled += updated.RowsAffected
			return nil
		})
		if err != nil {
			return reviewworkflow.OrphanReconciliation{Selected: len(runIDs), Reconciled: reconciled}, fmt.Errorf("중단된 리뷰 실행을 종료하지 못했습니다: %w", err)
		}
	}
	return reviewworkflow.OrphanReconciliation{Selected: len(runIDs), Reconciled: reconciled}, nil
}

func orphanExcludedRunStatuses() []string {
	return append(terminalRunStatuses(), string(reviewworkflow.RunStatusPublishing))
}
