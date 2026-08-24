package mysql

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/mapper"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ReviewWorkflowRepository struct {
	database *gorm.DB
}

const publicationRetryDelay = 5 * time.Minute

func NewReviewWorkflowRepository(database *gorm.DB) *ReviewWorkflowRepository {
	return &ReviewWorkflowRepository{database: database}
}

func (r *ReviewWorkflowRepository) CreateOrGetRun(ctx context.Context, run reviewworkflow.Run) (reviewworkflow.Run, error) {
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

func (r *ReviewWorkflowRepository) AcquireRun(ctx context.Context, runID uint64, startedAt time.Time, leaseExpiresAt time.Time) (string, error) {
	leaseToken, err := newLeaseToken()
	if err != nil {
		return "", fmt.Errorf("run lease를 만들지 못했습니다: %w", err)
	}
	superseded := false
	publicationLeased := false
	err = r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var run model.ReviewRun
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, runID).Error; err != nil {
			return err
		}
		leaseNow, err := databaseTime(transaction)
		if err != nil {
			return err
		}
		if reviewworkflow.RunStatus(run.Status).IsTerminal() {
			leaseToken = ""
			return nil
		}
		if run.LeaseToken != "" && run.LeaseExpiresAt != nil && run.LeaseExpiresAt.After(leaseNow) {
			return reviewworkflow.ErrRunLeased
		}
		registered, blockedByPublication, err := registerLatestRun(transaction, run, leaseNow)
		if err != nil {
			return err
		}
		if blockedByPublication {
			publicationLeased = true
			leaseToken = ""
			return nil
		}
		if !registered {
			superseded = true
			leaseToken = ""
			return nil
		}
		status := reviewworkflow.RunStatusRunning
		if reviewworkflow.RunStatus(run.Status) == reviewworkflow.RunStatusPublishing {
			status = reviewworkflow.RunStatusPublishing
		}
		updates := map[string]any{
			"status":           string(status),
			"lease_token":      leaseToken,
			"lease_expires_at": leaseExpiresAt,
		}
		if status != reviewworkflow.RunStatusPublishing {
			updates["heartbeat_at"] = startedAt
		}
		if err := transaction.Model(&model.ReviewRun{}).Where("id = ?", runID).Updates(updates).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("리뷰 실행 lease를 얻지 못했습니다: %w", err)
	}
	if superseded {
		return "", reviewworkflow.ErrRunSuperseded
	}
	if publicationLeased {
		return "", reviewworkflow.ErrPublicationLeased
	}
	return leaseToken, nil
}

func (r *ReviewWorkflowRepository) ResumeRun(ctx context.Context, runID uint64, leaseToken string, resumedAt time.Time) error {
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := requireRunLease(transaction, runID, leaseToken); err != nil {
			return err
		}
		updated := transaction.Model(&model.ReviewRun{}).
			Where("id = ? AND lease_token = ? AND status = ?", runID, leaseToken, string(reviewworkflow.RunStatusPublishing)).
			Updates(map[string]any{
				"status":       string(reviewworkflow.RunStatusRunning),
				"heartbeat_at": resumedAt,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return reviewworkflow.ErrRunLeased
		}
		return transaction.Model(&model.PullRequestState{}).
			Where("publishing_run_id = ? AND publishing_lease_token = ?", runID, leaseToken).
			UpdateColumns(publicationClaimClearValues()).Error
	})
	if err != nil {
		return fmt.Errorf("게시 조정 실행을 리뷰 상태로 되돌리지 못했습니다: %w", err)
	}
	return nil
}

func (r *ReviewWorkflowRepository) RenewRun(ctx context.Context, runID uint64, leaseToken string, heartbeatAt time.Time, leaseExpiresAt time.Time) error {
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var run model.ReviewRun
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, runID).Error; err != nil {
			return err
		}
		if err := validateRunLease(transaction, run, leaseToken); err != nil {
			return err
		}
		if _, err := requireLatestRun(transaction, run); err != nil {
			return err
		}
		updated := transaction.Model(&model.ReviewRun{}).
			Where("id = ? AND lease_token = ? AND status NOT IN ?", runID, leaseToken, terminalRunStatuses()).
			Updates(map[string]any{
				"heartbeat_at":     heartbeatAt,
				"lease_expires_at": leaseExpiresAt,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return reviewworkflow.ErrRunLeased
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("리뷰 실행 lease를 갱신하지 못했습니다: %w", err)
	}
	return nil
}

func (r *ReviewWorkflowRepository) ReleaseRun(ctx context.Context, runID uint64, leaseToken string, releasedAt time.Time) error {
	if leaseToken == "" {
		return nil
	}
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var run model.ReviewRun
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, runID).Error; err != nil {
			return err
		}
		if reviewworkflow.RunStatus(run.Status).IsTerminal() || run.LeaseToken != leaseToken {
			return nil
		}
		leaseNow, err := databaseTime(transaction)
		if err != nil {
			return err
		}
		leaseWasActive := run.LeaseExpiresAt != nil && run.LeaseExpiresAt.After(leaseNow)
		updates := map[string]any{
			"lease_token":      "",
			"lease_expires_at": nil,
		}
		if reviewworkflow.RunStatus(run.Status) != reviewworkflow.RunStatusPublishing {
			updates["heartbeat_at"] = releasedAt
		}
		if err := transaction.Model(&model.ReviewRun{}).Where("id = ? AND lease_token = ?", runID, leaseToken).Updates(updates).Error; err != nil {
			return err
		}
		if reviewworkflow.RunStatus(run.Status) != reviewworkflow.RunStatusPublishing || !leaseWasActive {
			return nil
		}
		return transaction.Model(&model.PullRequestState{}).
			Where("publishing_run_id = ? AND publishing_lease_token = ?", runID, leaseToken).
			Update("publishing_lease_expires_at", releasedAt.Add(publicationRetryDelay)).Error
	})
	if err != nil {
		return fmt.Errorf("리뷰 실행 lease를 해제하지 못했습니다: %w", err)
	}
	return nil
}

func (r *ReviewWorkflowRepository) ClaimPublication(ctx context.Context, runID uint64, runLeaseToken string, claimedAt time.Time, leaseExpiresAt time.Time) error {
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var run model.ReviewRun
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, runID).Error; err != nil {
			return err
		}
		if err := validateRunLease(transaction, run, runLeaseToken); err != nil {
			return err
		}
		state, err := requireLatestRun(transaction, run)
		if err != nil {
			return err
		}
		leaseNow, err := databaseTime(transaction)
		if err != nil {
			return err
		}
		claimActive := state.PublishingRunID != 0 && state.PublishingLeaseExpiresAt != nil && state.PublishingLeaseExpiresAt.After(leaseNow)
		claimOwned := state.PublishingRunID == run.ID && state.PublishingLeaseToken == runLeaseToken
		if claimActive && !claimOwned {
			return reviewworkflow.ErrPublicationLeased
		}
		if err := transaction.Model(&model.PullRequestState{}).Where("id = ?", state.ID).Updates(map[string]any{
			"publishing_run_id":           run.ID,
			"publishing_head_sha":         run.HeadSHA,
			"publishing_lease_token":      runLeaseToken,
			"publishing_lease_expires_at": leaseExpiresAt,
			"updated_at":                  claimedAt,
		}).Error; err != nil {
			return err
		}
		runUpdates := map[string]any{
			"status":           string(reviewworkflow.RunStatusPublishing),
			"lease_expires_at": leaseExpiresAt,
		}
		if reviewworkflow.RunStatus(run.Status) != reviewworkflow.RunStatusPublishing {
			runUpdates["heartbeat_at"] = claimedAt
		}
		updated := transaction.Model(&model.ReviewRun{}).
			Where("id = ? AND lease_token = ? AND status NOT IN ?", runID, runLeaseToken, terminalRunStatuses()).
			Updates(runUpdates)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return reviewworkflow.ErrRunLeased
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("리뷰 게시 권한을 얻지 못했습니다: %w", err)
	}
	return nil
}

func (r *ReviewWorkflowRepository) FinishRun(ctx context.Context, runID uint64, result reviewworkflow.RunResult) (reviewworkflow.RunStatus, error) {
	status := result.Status
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
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
			if !latest && status != reviewworkflow.RunStatusSuperseded {
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
		if result.PublicationInvalidation != nil {
			if err := persistPublicationInvalidation(transaction, run, *result.PublicationInvalidation, result.TerminalAt, result.ExpiresAt); err != nil {
				return err
			}
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
		if !summaryLoaded || coverageChanged {
			var summaryErr error
			summary, summaryErr = storedCoverageSummary(transaction, runID)
			if summaryErr != nil {
				return summaryErr
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
				"status":            string(status),
				"total_coverage":    summary.Total,
				"reviewed_coverage": summary.Reviewed,
				"failed_coverage":   summary.Failed,
				"deferred_coverage": summary.Deferred + summary.Pending,
				"skipped_coverage":  summary.Skipped,
				"error_summary":     boundedText(result.Error, 1000),
				"heartbeat_at":      result.TerminalAt,
				"terminal_at":       result.TerminalAt,
				"expires_at":        result.ExpiresAt,
				"lease_token":       "",
				"lease_expires_at":  nil,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errors.New("리뷰 실행 종료 상태가 충돌했습니다")
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("리뷰 실행 결과를 저장하지 못했습니다: %w", err)
	}
	return status, nil
}

func (r *ReviewWorkflowRepository) PublicationCandidateHighWatermark(ctx context.Context, before time.Time) (uint64, error) {
	var highWatermark uint64
	row := r.database.WithContext(ctx).Model(&model.ReviewRun{}).
		Where("status = ? AND heartbeat_at <= ?", string(reviewworkflow.RunStatusPublishing), before).
		Select("COALESCE(MAX(id), 0)").Row()
	if err := row.Scan(&highWatermark); err != nil {
		return 0, fmt.Errorf("게시 조정 대상 high watermark를 읽지 못했습니다: %w", err)
	}
	return highWatermark, nil
}

func (r *ReviewWorkflowRepository) PublicationCandidates(ctx context.Context, before time.Time, afterID uint64, throughID uint64, limit int) ([]reviewworkflow.Run, error) {
	if limit <= 0 {
		limit = 100
	}
	var entries []model.ReviewRun
	if err := r.database.WithContext(ctx).
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

func registerLatestRun(transaction *gorm.DB, run model.ReviewRun, registeredAt time.Time) (bool, bool, error) {
	var snapshotObservedAt *time.Time
	if !run.SnapshotObservedAt.IsZero() {
		snapshotObservedAt = &run.SnapshotObservedAt
	}
	state := model.PullRequestState{
		Owner:            run.Owner,
		Repository:       run.Repository,
		Number:           run.Number,
		LatestRunID:      run.ID,
		LatestHeadSHA:    run.HeadSHA,
		LatestObservedAt: snapshotObservedAt,
		LatestOrderKey:   run.SnapshotOrderKey,
		LatestRunAt:      &registeredAt,
		UpdatedAt:        registeredAt,
	}
	if err := transaction.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
		return false, false, err
	}
	state = model.PullRequestState{}
	if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("owner = ? AND repository = ? AND number = ?", run.Owner, run.Repository, run.Number).First(&state).Error; err != nil {
		return false, false, err
	}
	if latestRunIsNewer(state, run) {
		return false, false, nil
	}
	claimActive := state.PublishingRunID != 0 && state.PublishingLeaseExpiresAt != nil && state.PublishingLeaseExpiresAt.After(registeredAt)
	if claimActive {
		if state.LatestRunID != run.ID {
			updated := transaction.Model(&model.PullRequestState{}).Where("id = ?", state.ID).Updates(map[string]any{
				"latest_run_id":      run.ID,
				"latest_head_sha":    run.HeadSHA,
				"latest_observed_at": snapshotObservedAt,
				"latest_order_key":   run.SnapshotOrderKey,
				"latest_run_at":      registeredAt,
				"updated_at":         registeredAt,
			})
			if updated.Error != nil {
				return false, false, updated.Error
			}
			if updated.RowsAffected != 1 {
				return false, false, reviewworkflow.ErrPublicationLeased
			}
		}
		return false, true, nil
	}
	err := transaction.Model(&model.PullRequestState{}).Where("id = ?", state.ID).Updates(map[string]any{
		"latest_run_id":               run.ID,
		"latest_head_sha":             run.HeadSHA,
		"latest_observed_at":          snapshotObservedAt,
		"latest_order_key":            run.SnapshotOrderKey,
		"latest_run_at":               registeredAt,
		"publishing_run_id":           0,
		"publishing_head_sha":         "",
		"publishing_lease_token":      "",
		"publishing_lease_expires_at": nil,
		"updated_at":                  registeredAt,
	}).Error
	return err == nil, false, err
}

func latestRunIsNewer(state model.PullRequestState, run model.ReviewRun) bool {
	if state.LatestObservedAt == nil {
		return run.SnapshotObservedAt.IsZero() && state.LatestRunID > run.ID
	}
	if run.SnapshotObservedAt.IsZero() {
		return true
	}
	if state.LatestObservedAt.After(run.SnapshotObservedAt) {
		return true
	}
	if state.LatestObservedAt.Before(run.SnapshotObservedAt) {
		return false
	}
	if state.LatestOrderKey != run.SnapshotOrderKey && latestOrderKeysAreComparable(state.LatestOrderKey, run.SnapshotOrderKey) {
		return state.LatestOrderKey > run.SnapshotOrderKey
	}
	return state.LatestRunID > run.ID
}

func latestOrderKeysAreComparable(left string, right string) bool {
	leftScope, _, leftFound := strings.Cut(left, ":")
	rightScope, _, rightFound := strings.Cut(right, ":")
	if !leftFound || !rightFound || leftScope != rightScope {
		return false
	}
	return leftScope == "issue" || leftScope == "review"
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
