package mysql

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sort"
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
	entry := mapper.ToReviewRunModel(run)
	if err := r.database.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&entry).Error; err != nil {
		return reviewworkflow.Run{}, fmt.Errorf("리뷰 실행을 생성하지 못했습니다: %w", err)
	}
	entry = model.ReviewRun{}
	if err := r.database.WithContext(ctx).Where("run_key = ?", run.Key).First(&entry).Error; err != nil {
		return reviewworkflow.Run{}, fmt.Errorf("리뷰 실행을 읽지 못했습니다: %w", err)
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
		if reviewworkflow.RunStatus(run.Status).IsTerminal() {
			leaseToken = ""
			return nil
		}
		if run.LeaseToken != "" && run.LeaseExpiresAt != nil && run.LeaseExpiresAt.After(startedAt) {
			return reviewworkflow.ErrRunLeased
		}
		registered, blockedByPublication, err := registerLatestRun(transaction, run, startedAt)
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
		if reviewworkflow.RunStatus(run.Status).IsTerminal() || run.LeaseToken == "" || run.LeaseToken != leaseToken {
			return reviewworkflow.ErrRunLeased
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
		if reviewworkflow.RunStatus(run.Status) != reviewworkflow.RunStatusPublishing {
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

func (r *ReviewWorkflowRepository) SavePlan(ctx context.Context, runID uint64, runLeaseToken string, units []reviewworkflow.Unit, coverage []reviewworkflow.CoverageItem, plannedAt time.Time) ([]reviewworkflow.Unit, error) {
	stored := make([]reviewworkflow.Unit, 0, len(units))
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := requireRunLease(transaction, runID, runLeaseToken); err != nil {
			return err
		}
		var existingUnitHashes []string
		if err := transaction.Model(&model.ReviewUnit{}).Where("review_run_id = ?", runID).Pluck("unit_hash", &existingUnitHashes).Error; err != nil {
			return err
		}
		var existingCoverageKeys []string
		if err := transaction.Model(&model.CoverageItem{}).Where("review_run_id = ?", runID).Pluck("coverage_key", &existingCoverageKeys).Error; err != nil {
			return err
		}
		if len(existingUnitHashes) > 0 || len(existingCoverageKeys) > 0 {
			if !samePlan(existingUnitHashes, existingCoverageKeys, units, coverage) {
				return errors.New("이미 저장된 리뷰 계획과 새 계획이 다릅니다")
			}
		} else {
			for _, unit := range units {
				unit.RunID = runID
				entry := mapper.ToReviewUnitModel(unit)
				if err := transaction.Create(&entry).Error; err != nil {
					return err
				}
			}
		}

		var unitEntries []model.ReviewUnit
		if err := transaction.Where("review_run_id = ?", runID).Order("ordinal ASC").Find(&unitEntries).Error; err != nil {
			return err
		}
		unitIDs := make(map[string]uint64, len(unitEntries))
		for _, entry := range unitEntries {
			unitIDs[entry.UnitHash] = entry.ID
			stored = append(stored, mapper.ToReviewUnit(entry))
		}

		if len(existingCoverageKeys) == 0 {
			entries := make([]model.CoverageItem, 0, len(coverage))
			for _, item := range coverage {
				item.RunID = runID
				if item.UnitHash != "" {
					unitID, found := unitIDs[item.UnitHash]
					if !found {
						return fmt.Errorf("coverage unit %s를 찾지 못했습니다", item.UnitHash)
					}
					item.UnitID = &unitID
				}
				entries = append(entries, mapper.ToCoverageItemModel(item))
			}
			if len(entries) > 0 {
				if err := transaction.CreateInBatches(&entries, 100).Error; err != nil {
					return err
				}
			}
		}
		updated := transaction.Model(&model.ReviewRun{}).
			Where("id = ? AND lease_token = ? AND status IN ?", runID, runLeaseToken, []string{string(reviewworkflow.RunStatusPlanning), string(reviewworkflow.RunStatusRunning)}).
			Updates(map[string]any{
				"status":       string(reviewworkflow.RunStatusRunning),
				"heartbeat_at": plannedAt,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errors.New("종료되었거나 충돌한 리뷰 실행에는 계획을 저장할 수 없습니다")
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("리뷰 실행 계획을 저장하지 못했습니다: %w", err)
	}
	return stored, nil
}

func (r *ReviewWorkflowRepository) StartUnit(ctx context.Context, runID uint64, runLeaseToken string, unitHash string, startedAt time.Time, leaseExpiresAt time.Time) (string, error) {
	leaseToken, err := newLeaseToken()
	if err != nil {
		return "", fmt.Errorf("unit lease를 만들지 못했습니다: %w", err)
	}
	err = r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := requireRunLease(transaction, runID, runLeaseToken); err != nil {
			return err
		}
		if err := transaction.Model(&model.ReviewRun{}).Where("id = ? AND lease_token = ?", runID, runLeaseToken).Updates(map[string]any{
			"heartbeat_at":     startedAt,
			"lease_expires_at": leaseExpiresAt,
		}).Error; err != nil {
			return err
		}
		updates := map[string]any{
			"status":           string(reviewworkflow.UnitStatusRunning),
			"attempt_count":    gorm.Expr("attempt_count + 1"),
			"started_at":       startedAt,
			"finished_at":      nil,
			"heartbeat_at":     startedAt,
			"error_summary":    "",
			"lease_token":      leaseToken,
			"lease_expires_at": leaseExpiresAt,
		}
		result := transaction.Model(&model.ReviewUnit{}).
			Where("review_run_id = ? AND unit_hash = ?", runID, unitHash).
			Where("status IN ? OR (status = ? AND (lease_expires_at IS NULL OR lease_expires_at <= ?))", []string{
				string(reviewworkflow.UnitStatusPending),
				string(reviewworkflow.UnitStatusSucceeded),
				string(reviewworkflow.UnitStatusFailed),
			}, string(reviewworkflow.UnitStatusRunning), startedAt).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("unit을 실행 가능한 상태로 전이하지 못했습니다")
		}
		var unit model.ReviewUnit
		if err := transaction.Where("review_run_id = ? AND unit_hash = ?", runID, unitHash).First(&unit).Error; err != nil {
			return err
		}
		return transaction.Model(&model.CoverageItem{}).
			Where("review_run_id = ? AND review_unit_id = ? AND status IN ?", runID, unit.ID, []string{
				string(reviewworkflow.CoverageStatusReviewed),
				string(reviewworkflow.CoverageStatusFailed),
			}).Updates(map[string]any{
			"status":      string(reviewworkflow.CoverageStatusPlanned),
			"reviewed_at": nil,
		}).Error
	})
	if err != nil {
		return "", fmt.Errorf("리뷰 unit을 시작하지 못했습니다: %w", err)
	}
	return leaseToken, nil
}

func (r *ReviewWorkflowRepository) FinishUnit(ctx context.Context, runID uint64, runLeaseToken string, unitHash string, leaseToken string, result reviewworkflow.UnitResult) error {
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := requireRunLease(transaction, runID, runLeaseToken); err != nil {
			return err
		}
		var unit model.ReviewUnit
		if err := transaction.Where("review_run_id = ? AND unit_hash = ? AND lease_token = ?", runID, unitHash, leaseToken).First(&unit).Error; err != nil {
			return err
		}
		updated := transaction.Model(&model.ReviewUnit{}).
			Where("id = ? AND status = ? AND lease_token = ?", unit.ID, string(reviewworkflow.UnitStatusRunning), leaseToken).
			Updates(map[string]any{
				"status":            string(result.Status),
				"provider":          result.Provider,
				"model":             result.Model,
				"prompt_tokens":     result.Usage.PromptTokens,
				"completion_tokens": result.Usage.CompletionTokens,
				"total_tokens":      result.Usage.TotalTokens,
				"error_summary":     boundedText(result.Error, 1000),
				"finished_at":       result.FinishedAt,
				"heartbeat_at":      result.FinishedAt,
				"lease_expires_at":  nil,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errors.New("unit 완료 상태가 충돌했습니다")
		}
		coverageStatus := reviewworkflow.CoverageStatusFailed
		var reviewedAt *time.Time
		if result.Status == reviewworkflow.UnitStatusSucceeded {
			coverageStatus = reviewworkflow.CoverageStatusReviewed
			reviewedAt = &result.FinishedAt
		}
		return transaction.Model(&model.CoverageItem{}).
			Where("review_run_id = ? AND review_unit_id = ? AND status = ?", runID, unit.ID, string(reviewworkflow.CoverageStatusPlanned)).
			Updates(map[string]any{
				"status":      string(coverageStatus),
				"reviewed_at": reviewedAt,
			}).Error
	})
	if err != nil {
		return fmt.Errorf("리뷰 unit 결과를 저장하지 못했습니다: %w", err)
	}
	return nil
}

func (r *ReviewWorkflowRepository) ClaimPublication(ctx context.Context, runID uint64, runLeaseToken string, claimedAt time.Time, leaseExpiresAt time.Time) error {
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var run model.ReviewRun
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, runID).Error; err != nil {
			return err
		}
		if reviewworkflow.RunStatus(run.Status).IsTerminal() || run.LeaseToken == "" || run.LeaseToken != runLeaseToken {
			return reviewworkflow.ErrRunLeased
		}
		state, err := requireLatestRun(transaction, run)
		if err != nil {
			return err
		}
		claimActive := state.PublishingRunID != 0 && state.PublishingLeaseExpiresAt != nil && state.PublishingLeaseExpiresAt.After(claimedAt)
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
		finishLeaseToken := result.LeaseToken
		expiredSupersede := result.Status == reviewworkflow.RunStatusSuperseded && result.LeaseToken == "" && (run.LeaseToken == "" || run.LeaseExpiresAt == nil || !run.LeaseExpiresAt.After(result.TerminalAt))
		if run.LeaseToken != result.LeaseToken {
			if !expiredSupersede {
				return reviewworkflow.ErrRunLeased
			}
			finishLeaseToken = run.LeaseToken
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
			status = summary.TerminalStatus()
		}
		var state model.PullRequestState
		stateErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("owner = ? AND repository = ? AND number = ?", run.Owner, run.Repository, run.Number).First(&state).Error
		if stateErr != nil && !errors.Is(stateErr, gorm.ErrRecordNotFound) {
			return stateErr
		}
		if errors.Is(stateErr, gorm.ErrRecordNotFound) && reviewworkflow.RunStatus(run.Status) == reviewworkflow.RunStatusPublishing {
			return reviewworkflow.ErrPublicationLeased
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

func (r *ReviewWorkflowRepository) ReconcileOrphans(ctx context.Context, staleBefore time.Time, terminalAt time.Time, expiresAt time.Time, limit int) (reviewworkflow.OrphanReconciliation, error) {
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
					"error_summary":    "7일 동안 진행되지 않아 종료됨",
					"finished_at":      terminalAt,
					"heartbeat_at":     terminalAt,
					"lease_token":      "",
					"lease_expires_at": nil,
				}).Error; err != nil {
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

func (r *ReviewWorkflowRepository) DeleteExpired(ctx context.Context, now time.Time, legacyCutoff time.Time, limit int) (reviewworkflow.CleanupResult, error) {
	if limit <= 0 {
		limit = 500
	}
	cleaned := reviewworkflow.CleanupResult{}
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var invalidationIDs []uint64
		if err := transaction.Model(&model.PublicationInvalidation{}).
			Where("expires_at <= ?", now).
			Order("id ASC").Limit(limit).Pluck("id", &invalidationIDs).Error; err != nil {
			return err
		}
		if len(invalidationIDs) > 0 {
			deletedInvalidations := transaction.Where("id IN ? AND expires_at <= ?", invalidationIDs, now).Delete(&model.PublicationInvalidation{})
			if deletedInvalidations.Error != nil {
				return deletedInvalidations.Error
			}
			cleaned.Publications += deletedInvalidations.RowsAffected
		}

		var replyPublicationIDs []uint64
		if err := transaction.Model(&model.ReplyPublication{}).
			Where("expires_at <= ?", now).
			Where("completed_at IS NOT NULL OR lease_token = '' OR lease_expires_at IS NULL OR lease_expires_at <= ?", now).
			Order("id ASC").Limit(limit).Pluck("id", &replyPublicationIDs).Error; err != nil {
			return err
		}
		if len(replyPublicationIDs) > 0 {
			deletedPublications := transaction.Where("id IN ? AND expires_at <= ?", replyPublicationIDs, now).
				Where("completed_at IS NOT NULL OR lease_token = '' OR lease_expires_at IS NULL OR lease_expires_at <= ?", now).
				Delete(&model.ReplyPublication{})
			if deletedPublications.Error != nil {
				return deletedPublications.Error
			}
			cleaned.Publications += deletedPublications.RowsAffected
		}

		var runIDs []uint64
		if err := transaction.Model(&model.ReviewRun{}).Where("expires_at IS NOT NULL AND expires_at <= ?", now).Order("id ASC").Limit(limit).Pluck("id", &runIDs).Error; err != nil {
			return err
		}
		if len(runIDs) > 0 {
			var linkedReviewIDs []uint64
			if err := transaction.Model(&model.Review{}).Where("review_run_id IN ?", runIDs).Pluck("id", &linkedReviewIDs).Error; err != nil {
				return err
			}
			if len(linkedReviewIDs) > 0 {
				deletedFindings := transaction.Where("review_id IN ?", linkedReviewIDs).Delete(&model.Finding{})
				if deletedFindings.Error != nil {
					return deletedFindings.Error
				}
				cleaned.Findings += deletedFindings.RowsAffected
				deletedReviews := transaction.Where("id IN ?", linkedReviewIDs).Delete(&model.Review{})
				if deletedReviews.Error != nil {
					return deletedReviews.Error
				}
				cleaned.Reviews += deletedReviews.RowsAffected
			}
			if err := transaction.Where("review_run_id IN ?", runIDs).Delete(&model.CoverageItem{}).Error; err != nil {
				return err
			}
			if err := transaction.Where("review_run_id IN ?", runIDs).Delete(&model.ReviewUnit{}).Error; err != nil {
				return err
			}
			deleted := transaction.Where("id IN ?", runIDs).Delete(&model.ReviewRun{})
			if deleted.Error != nil {
				return deleted.Error
			}
			cleaned.Runs = deleted.RowsAffected
		}

		var reviewIDs []uint64
		if err := transaction.Model(&model.Review{}).Where("review_run_id IS NULL AND finished_at <= ?", legacyCutoff).Order("id ASC").Limit(limit).Pluck("id", &reviewIDs).Error; err != nil {
			return err
		}
		if len(reviewIDs) > 0 {
			deletedFindings := transaction.Where("review_id IN ?", reviewIDs).Delete(&model.Finding{})
			if deletedFindings.Error != nil {
				return deletedFindings.Error
			}
			cleaned.Findings += deletedFindings.RowsAffected
			deletedReviews := transaction.Where("id IN ?", reviewIDs).Delete(&model.Review{})
			if deletedReviews.Error != nil {
				return deletedReviews.Error
			}
			cleaned.Reviews += deletedReviews.RowsAffected
		}
		var orphanFindingIDs []uint64
		orphanFindingCondition := "created_at <= ? AND (review_id = 0 OR NOT EXISTS (SELECT 1 FROM reviews WHERE reviews.id = findings.review_id))"
		if err := transaction.Model(&model.Finding{}).Where(orphanFindingCondition, legacyCutoff).Order("id ASC").Limit(limit).Pluck("id", &orphanFindingIDs).Error; err != nil {
			return err
		}
		if len(orphanFindingIDs) > 0 {
			deletedOrphans := transaction.Where("id IN ?", orphanFindingIDs).Where(orphanFindingCondition, legacyCutoff).Delete(&model.Finding{})
			if deletedOrphans.Error != nil {
				return deletedOrphans.Error
			}
			cleaned.Findings += deletedOrphans.RowsAffected
		}

		var expiredClaimStateIDs []uint64
		if err := transaction.Model(&model.PullRequestState{}).
			Where("COALESCE(publishing_run_id, 0) <> 0 AND (publishing_lease_expires_at IS NULL OR publishing_lease_expires_at <= ?)", now).
			Order("id ASC").Limit(limit).Pluck("id", &expiredClaimStateIDs).Error; err != nil {
			return err
		}
		if len(expiredClaimStateIDs) > 0 {
			clearedClaims := transaction.Model(&model.PullRequestState{}).
				Where("id IN ? AND COALESCE(publishing_run_id, 0) <> 0 AND (publishing_lease_expires_at IS NULL OR publishing_lease_expires_at <= ?)", expiredClaimStateIDs, now).
				UpdateColumns(publicationClaimClearValues())
			if clearedClaims.Error != nil {
				return clearedClaims.Error
			}
			cleaned.States += clearedClaims.RowsAffected
		}

		var expiredSummaryStateIDs []uint64
		if err := transaction.Model(&model.PullRequestState{}).
			Where("summary_operation_key <> '' AND summary_expires_at IS NOT NULL AND summary_expires_at <= ?", now).
			Where("summary_publishing_key = '' OR summary_lease_expires_at IS NULL OR summary_lease_expires_at <= ?", now).
			Order("id ASC").Limit(limit).Pluck("id", &expiredSummaryStateIDs).Error; err != nil {
			return err
		}
		if len(expiredSummaryStateIDs) > 0 {
			clearedSummaries := transaction.Model(&model.PullRequestState{}).
				Where("id IN ? AND summary_operation_key <> '' AND summary_expires_at IS NOT NULL AND summary_expires_at <= ?", expiredSummaryStateIDs, now).
				Where("summary_publishing_key = '' OR summary_lease_expires_at IS NULL OR summary_lease_expires_at <= ?", now).
				UpdateColumns(summaryPublicationClearValues())
			if clearedSummaries.Error != nil {
				return clearedSummaries.Error
			}
			cleaned.States += clearedSummaries.RowsAffected
		}

		watermarkStateIDs, err := expiredWatermarkStateIDs(transaction, legacyCutoff, limit)
		if err != nil {
			return err
		}
		if len(watermarkStateIDs) > 0 {
			clearedStates := transaction.Model(&model.PullRequestState{}).
				Where("id IN ? AND last_reviewed_sha <> '' AND (last_reviewed_at IS NULL OR last_reviewed_at <= ?)", watermarkStateIDs, legacyCutoff).
				UpdateColumns(map[string]any{
					"last_reviewed_sha":    "",
					"last_reviewed_run_id": 0,
					"last_reviewed_at":     nil,
				})
			if clearedStates.Error != nil {
				return clearedStates.Error
			}
			cleaned.States += clearedStates.RowsAffected
		}
		var latestStateIDs []uint64
		if err := transaction.Model(&model.PullRequestState{}).
			Where("COALESCE(latest_run_id, 0) <> 0 AND latest_run_at IS NOT NULL AND latest_run_at <= ?", legacyCutoff).
			Order("id ASC").Limit(limit).Pluck("id", &latestStateIDs).Error; err != nil {
			return err
		}
		if len(latestStateIDs) > 0 {
			clearedLatest := transaction.Model(&model.PullRequestState{}).
				Where("id IN ? AND COALESCE(latest_run_id, 0) <> 0 AND latest_run_at IS NOT NULL AND latest_run_at <= ?", latestStateIDs, legacyCutoff).
				UpdateColumns(map[string]any{
					"latest_run_id":      0,
					"latest_head_sha":    "",
					"latest_observed_at": nil,
					"latest_order_key":   "",
					"latest_run_at":      nil,
				})
			if clearedLatest.Error != nil {
				return clearedLatest.Error
			}
			cleaned.States += clearedLatest.RowsAffected
		}

		var deletableStateIDs []uint64
		if err := transaction.Model(&model.PullRequestState{}).
			Where("last_reviewed_sha = '' AND COALESCE(latest_run_id, 0) = 0 AND COALESCE(publishing_run_id, 0) = 0 AND summary_operation_key = '' AND summary_publishing_key = '' AND updated_at <= ?", legacyCutoff).
			Order("id ASC").Limit(limit).Pluck("id", &deletableStateIDs).Error; err != nil {
			return err
		}
		if len(deletableStateIDs) > 0 {
			deletedStates := transaction.
				Where("id IN ? AND last_reviewed_sha = '' AND COALESCE(latest_run_id, 0) = 0 AND COALESCE(publishing_run_id, 0) = 0 AND summary_operation_key = '' AND summary_publishing_key = '' AND updated_at <= ?", deletableStateIDs, legacyCutoff).
				Delete(&model.PullRequestState{})
			if deletedStates.Error != nil {
				return deletedStates.Error
			}
			cleaned.States += deletedStates.RowsAffected
		}
		return nil
	})
	if err != nil {
		return reviewworkflow.CleanupResult{}, fmt.Errorf("만료된 리뷰 데이터를 제거하지 못했습니다: %w", err)
	}
	return cleaned, nil
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

func requireRunLease(transaction *gorm.DB, runID uint64, leaseToken string) error {
	var run model.ReviewRun
	if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, runID).Error; err != nil {
		return err
	}
	if reviewworkflow.RunStatus(run.Status).IsTerminal() || run.LeaseToken == "" || run.LeaseToken != leaseToken {
		return reviewworkflow.ErrRunLeased
	}
	_, err := requireLatestRun(transaction, run)
	return err
}

func requireLatestRun(transaction *gorm.DB, run model.ReviewRun) (model.PullRequestState, error) {
	var state model.PullRequestState
	if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("owner = ? AND repository = ? AND number = ?", run.Owner, run.Repository, run.Number).First(&state).Error; err != nil {
		return model.PullRequestState{}, err
	}
	if state.LatestRunID != run.ID || state.LatestHeadSHA != run.HeadSHA {
		return model.PullRequestState{}, reviewworkflow.ErrRunSuperseded
	}
	return state, nil
}

func clearPublicationClaim(transaction *gorm.DB, stateID uint64, runID uint64, leaseToken string) error {
	result := transaction.Model(&model.PullRequestState{}).
		Where("id = ? AND publishing_run_id = ? AND publishing_lease_token = ?", stateID, runID, leaseToken).
		UpdateColumns(publicationClaimClearValues())
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return reviewworkflow.ErrPublicationLeased
	}
	return nil
}

func publicationClaimClearValues() map[string]any {
	return map[string]any{
		"publishing_run_id":           0,
		"publishing_head_sha":         "",
		"publishing_lease_token":      "",
		"publishing_lease_expires_at": nil,
	}
}

func summaryPublicationClearValues() map[string]any {
	return map[string]any{
		"summary_operation_key":    "",
		"summary_order_key":        "",
		"summary_observed_at":      nil,
		"summary_publishing_key":   "",
		"summary_lease_token":      "",
		"summary_lease_expires_at": nil,
		"summary_completed_key":    "",
		"summary_completed_at":     nil,
		"summary_expires_at":       nil,
	}
}

func expiredWatermarkStateIDs(transaction *gorm.DB, cutoff time.Time, limit int) ([]uint64, error) {
	stateIDs := make([]uint64, 0, limit)
	if err := transaction.Model(&model.PullRequestState{}).
		Where("last_reviewed_sha <> '' AND last_reviewed_at IS NOT NULL AND last_reviewed_at <= ?", cutoff).
		Order("id ASC").Limit(limit).Pluck("id", &stateIDs).Error; err != nil {
		return nil, err
	}
	remaining := limit - len(stateIDs)
	if remaining <= 0 {
		return stateIDs, nil
	}
	legacyStateIDs := make([]uint64, 0, remaining)
	if err := transaction.Model(&model.PullRequestState{}).
		Where("last_reviewed_sha <> '' AND last_reviewed_at IS NULL").
		Order("id ASC").Limit(remaining).Pluck("id", &legacyStateIDs).Error; err != nil {
		return nil, err
	}
	return append(stateIDs, legacyStateIDs...), nil
}

func boundedText(value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}
	return string(runes[:limit])
}

func newLeaseToken() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func terminalRunStatuses() []string {
	return []string{
		string(reviewworkflow.RunStatusComplete),
		string(reviewworkflow.RunStatusPartial),
		string(reviewworkflow.RunStatusFailed),
		string(reviewworkflow.RunStatusSuperseded),
		string(reviewworkflow.RunStatusCancelled),
		string(reviewworkflow.RunStatusSkipped),
	}
}

func orphanExcludedRunStatuses() []string {
	return append(terminalRunStatuses(), string(reviewworkflow.RunStatusPublishing))
}

func samePlan(storedUnits []string, storedCoverage []string, units []reviewworkflow.Unit, coverage []reviewworkflow.CoverageItem) bool {
	expectedUnits := make([]string, 0, len(units))
	for _, unit := range units {
		expectedUnits = append(expectedUnits, unit.Hash)
	}
	expectedCoverage := make([]string, 0, len(coverage))
	for _, item := range coverage {
		expectedCoverage = append(expectedCoverage, item.Key)
	}
	sort.Strings(storedUnits)
	sort.Strings(storedCoverage)
	sort.Strings(expectedUnits)
	sort.Strings(expectedCoverage)
	return slices.Equal(storedUnits, expectedUnits) && slices.Equal(storedCoverage, expectedCoverage)
}
