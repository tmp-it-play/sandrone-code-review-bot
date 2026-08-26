package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/mapper"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *ReviewRunStore) FinishRun(ctx context.Context, runID uint64, result reviewworkflow.RunResult) (reviewworkflow.RunStatus, error) {
	status := result.Status
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		return finishReviewRun(transaction, runID, result, &status)
	})
	if err != nil {
		return "", fmt.Errorf("리뷰 실행 결과를 저장하지 못했습니다: %w", err)
	}
	return status, nil
}

func finishReviewRun(transaction *gorm.DB, runID uint64, result reviewworkflow.RunResult, resolvedStatus *reviewworkflow.RunStatus) error {
	status := result.Status
	defer func() {
		*resolvedStatus = status
	}()
	var run model.ReviewRun
	if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, runID).Error; err != nil {
		return err
	}
	if reviewworkflow.RunStatus(run.Status).IsTerminal() {
		status = reviewworkflow.RunStatus(run.Status)
		return nil
	}
	leaseNow, err := databaseTime(transaction)
	if err != nil {
		return err
	}
	finishLeaseToken := result.LeaseToken
	expiredSupersede := result.Status == reviewworkflow.RunStatusSuperseded && result.LeaseToken == "" && (run.LeaseToken == "" || run.LeaseExpiresAt == nil || !run.LeaseExpiresAt.After(leaseNow))
	if run.LeaseToken != result.LeaseToken {
		if !expiredSupersede {
			return reviewworkflow.ErrRunLeased
		}
		finishLeaseToken = run.LeaseToken
	} else if result.LeaseToken != "" && (run.LeaseExpiresAt == nil || !run.LeaseExpiresAt.After(leaseNow)) {
		return reviewworkflow.ErrRunLeased
	}
	status = result.Status
	summary := reviewworkflow.CoverageSummary{}
	summaryLoaded := false
	if status == reviewworkflow.RunStatusComplete || status == reviewworkflow.RunStatusPartial || status == reviewworkflow.RunStatusSkipped {
		var summaryErr error
		summary, summaryErr = storedCoverageSummary(transaction, runID)
		if summaryErr != nil {
			return summaryErr
		}
		summaryLoaded = true
		if summary.Pending > 0 {
			return fmt.Errorf("종료할 리뷰 실행에 미완료 coverage %d개가 남아 있습니다", summary.Pending)
		}
		coverageStatus := summary.TerminalStatus()
		if result.Status == reviewworkflow.RunStatusPartial && coverageStatus == reviewworkflow.RunStatusComplete {
			status = reviewworkflow.RunStatusPartial
		} else {
			status = coverageStatus
		}
	}
	var state model.PullRequestState
	stateErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("owner = ? AND repository = ? AND number = ?", run.Owner, run.Repository, run.Number).First(&state).Error
	if stateErr != nil && !errors.Is(stateErr, gorm.ErrRecordNotFound) {
		return stateErr
	}
	if errors.Is(stateErr, gorm.ErrRecordNotFound) && reviewworkflow.RunStatus(run.Status) == reviewworkflow.RunStatusPublishing {
		return reviewworkflow.ErrPublicationLeased
	}
	if reviewworkflow.RunStatus(run.Status) == reviewworkflow.RunStatusPublishing && (result.Status == reviewworkflow.RunStatusComplete || result.Status == reviewworkflow.RunStatusPartial) {
		publication, err := requireCompletedReviewPublication(transaction, runID, leaseNow)
		if err != nil {
			return err
		}
		if publication.PayloadHash != "" {
			intended := publication.Finalization
			if result.Status != intended.Status || result.Error != intended.Detail || result.AdvanceWatermark != intended.AdvanceWatermark {
				return errors.New("리뷰 게시 완료 metadata가 canonical publication과 일치하지 않습니다")
			}
		}
	}
	if stateErr == nil {
		latest := state.LatestRunID == run.ID && state.LatestHeadSHA == run.HeadSHA
		claimLeaseToken := ""
		if !latest && status != reviewworkflow.RunStatusSuperseded && !result.DetachedFinalization {
			if reviewworkflow.RunStatus(run.Status) == reviewworkflow.RunStatusPublishing {
				return reviewworkflow.ErrRunSuperseded
			}
			status = reviewworkflow.RunStatusSuperseded
		}
		if reviewworkflow.RunStatus(run.Status) == reviewworkflow.RunStatusPublishing {
			claimOwned := state.PublishingRunID == run.ID && state.PublishingHeadSHA == run.HeadSHA && state.PublishingLeaseToken == result.LeaseToken
			expiredClaim := expiredSupersede && state.PublishingRunID == run.ID && (state.PublishingLeaseExpiresAt == nil || !state.PublishingLeaseExpiresAt.After(result.TerminalAt))
			claimMissing := expiredSupersede && state.PublishingRunID == 0
			if !claimOwned && !expiredClaim && !claimMissing {
				if !latest {
					status = reviewworkflow.RunStatusSuperseded
				} else {
					return reviewworkflow.ErrPublicationLeased
				}
			}
			if claimOwned {
				claimLeaseToken = result.LeaseToken
			} else if expiredClaim {
				claimLeaseToken = state.PublishingLeaseToken
			}
		}
		if result.AdvanceWatermark && latest && (status == reviewworkflow.RunStatusComplete || status == reviewworkflow.RunStatusSkipped) {
			if err := updateWatermark(transaction, state.ID, run, result.TerminalAt); err != nil {
				return err
			}
		}
		if claimLeaseToken != "" && state.PublishingRunID == run.ID && state.PublishingLeaseToken == claimLeaseToken {
			if err := clearPublicationClaim(transaction, state.ID, run.ID, claimLeaseToken); err != nil {
				return err
			}
		}
	}
	invalidation := result.PublicationInvalidation
	if invalidation == nil && result.TerminalProgress != nil {
		invalidation = result.TerminalProgress.Invalidation(status, result.TerminalAt, result.ExpiresAt)
	}
	if invalidation != nil {
		if err := persistPublicationInvalidation(transaction, run, *invalidation, result.TerminalAt, result.ExpiresAt); err != nil {
			return err
		}
	}
	if err := stopProgressCommentRefreshForUpdate(transaction, run, leaseNow); err != nil {
		return err
	}
	coverageChanged := false
	switch status {
	case reviewworkflow.RunStatusFailed:
		if err := transaction.Model(&model.ReviewUnit{}).
			Where("review_run_id = ? AND status IN ?", runID, []string{
				string(reviewworkflow.UnitStatusPending),
				string(reviewworkflow.UnitStatusRunning),
			}).Updates(map[string]any{
			"status":           string(reviewworkflow.UnitStatusFailed),
			"retryable":        false,
			"retry_at":         nil,
			"error_summary":    boundedText(result.Error, 1000),
			"finished_at":      result.TerminalAt,
			"heartbeat_at":     result.TerminalAt,
			"lease_token":      "",
			"lease_expires_at": nil,
		}).Error; err != nil {
			return err
		}
		if err := transaction.Model(&model.CoverageItem{}).
			Where("review_run_id = ? AND status IN ?", runID, []string{
				string(reviewworkflow.CoverageStatusIndexed),
				string(reviewworkflow.CoverageStatusPlanned),
			}).Updates(map[string]any{
			"status": string(reviewworkflow.CoverageStatusFailed),
			"reason": "run_failed",
		}).Error; err != nil {
			return err
		}
		coverageChanged = true
	case reviewworkflow.RunStatusSuperseded, reviewworkflow.RunStatusCancelled:
		reason := "run_superseded"
		if status == reviewworkflow.RunStatusCancelled {
			reason = "run_cancelled"
		}
		if err := transaction.Model(&model.ReviewUnit{}).
			Where("review_run_id = ? AND status IN ?", runID, []string{
				string(reviewworkflow.UnitStatusPending),
				string(reviewworkflow.UnitStatusRunning),
			}).Updates(map[string]any{
			"status":           string(reviewworkflow.UnitStatusDeferred),
			"retryable":        false,
			"retry_at":         nil,
			"error_summary":    reason,
			"finished_at":      result.TerminalAt,
			"heartbeat_at":     result.TerminalAt,
			"lease_token":      "",
			"lease_expires_at": nil,
		}).Error; err != nil {
			return err
		}
		coverageStatus := reviewworkflow.CoverageStatusSuperseded
		if status == reviewworkflow.RunStatusCancelled {
			coverageStatus = reviewworkflow.CoverageStatusDeferred
		}
		if err := transaction.Model(&model.CoverageItem{}).
			Where("review_run_id = ? AND status IN ?", runID, []string{
				string(reviewworkflow.CoverageStatusIndexed),
				string(reviewworkflow.CoverageStatusPlanned),
			}).Updates(map[string]any{
			"status": string(coverageStatus),
			"reason": reason,
		}).Error; err != nil {
			return err
		}
		coverageChanged = true
	}
	if err := transaction.Model(&model.ReviewUnit{}).
		Where("review_run_id = ? AND retryable = ?", runID, true).
		Updates(map[string]any{"retryable": false, "retry_at": nil}).Error; err != nil {
		return err
	}
	if !summaryLoaded || coverageChanged {
		var summaryErr error
		summary, summaryErr = storedCoverageSummary(transaction, runID)
		if summaryErr != nil {
			return summaryErr
		}
	}
	supersedingHeadSHA := ""
	if status == reviewworkflow.RunStatusSuperseded {
		supersedingHeadSHA = result.SupersedingHeadSHA
		if supersedingHeadSHA == run.HeadSHA {
			supersedingHeadSHA = ""
		}
		if supersedingHeadSHA == "" && stateErr == nil && state.LatestHeadSHA != run.HeadSHA {
			supersedingHeadSHA = state.LatestHeadSHA
		}
	}
	projectionOutcome := reviewOutcomeForRunStatus(status)
	if status == result.Status && result.ReviewOutcome != "" {
		projectionOutcome = result.ReviewOutcome
	}
	if err := updateReviewProjection(transaction, run, projectionOutcome, result.Error, result.TerminalAt); err != nil {
		return err
	}
	updated := transaction.Model(&model.ReviewRun{}).
		Where("id = ? AND lease_token = ? AND status NOT IN ?", runID, finishLeaseToken, terminalRunStatuses()).
		Updates(map[string]any{
			"status":               string(status),
			"total_coverage":       summary.Total,
			"reviewed_coverage":    summary.Reviewed,
			"failed_coverage":      summary.Failed,
			"deferred_coverage":    summary.Deferred + summary.Pending,
			"skipped_coverage":     summary.Skipped,
			"error_summary":        boundedText(result.Error, 1000),
			"superseding_head_sha": supersedingHeadSHA,
			"heartbeat_at":         result.TerminalAt,
			"terminal_at":          result.TerminalAt,
			"expires_at":           result.ExpiresAt,
			"lease_token":          "",
			"lease_expires_at":     nil,
		})
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected != 1 {
		return errors.New("리뷰 실행 종료 상태가 충돌했습니다")
	}
	return nil
}

func storedCoverageSummary(database *gorm.DB, runID uint64) (reviewworkflow.CoverageSummary, error) {
	var entries []model.CoverageItem
	if err := database.Where("review_run_id = ?", runID).Find(&entries).Error; err != nil {
		return reviewworkflow.CoverageSummary{}, err
	}
	items := make([]reviewworkflow.CoverageItem, 0, len(entries))
	for _, entry := range entries {
		items = append(items, mapper.ToCoverageItem(entry))
	}
	return reviewworkflow.SummarizeCoverage(items), nil
}

func updateWatermark(transaction *gorm.DB, stateID uint64, run model.ReviewRun, updatedAt time.Time) error {
	return transaction.Model(&model.PullRequestState{}).Where("id = ?", stateID).Updates(map[string]any{
		"last_reviewed_sha":    run.HeadSHA,
		"last_reviewed_run_id": run.ID,
		"last_reviewed_at":     updatedAt,
		"updated_at":           updatedAt,
	}).Error
}

func reviewOutcomeForRunStatus(status reviewworkflow.RunStatus) review.Outcome {
	switch status {
	case reviewworkflow.RunStatusComplete:
		return review.OutcomeSucceeded
	case reviewworkflow.RunStatusPartial:
		return review.OutcomePartial
	case reviewworkflow.RunStatusSuperseded:
		return review.OutcomeSuperseded
	case reviewworkflow.RunStatusSkipped:
		return review.OutcomeSkipped
	case reviewworkflow.RunStatusFailed, reviewworkflow.RunStatusCancelled:
		return review.OutcomeFailed
	default:
		return review.OutcomeUnavailable
	}
}
