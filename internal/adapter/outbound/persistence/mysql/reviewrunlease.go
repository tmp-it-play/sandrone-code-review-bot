package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const publicationRetryDelay = 5 * time.Minute

func (r *ReviewRunStore) AcquireRun(ctx context.Context, runID uint64, startedAt time.Time, leaseExpiresAt time.Time) (reviewworkflow.RunLease, error) {
	leaseToken, err := newLeaseToken()
	if err != nil {
		return reviewworkflow.RunLease{}, fmt.Errorf("run lease를 만들지 못했습니다: %w", err)
	}
	lease := reviewworkflow.RunLease{Token: leaseToken}
	superseded := false
	publicationLeaseUntil := time.Time{}
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
			lease.Token = ""
			return nil
		}
		if run.LeaseToken != "" && run.LeaseExpiresAt != nil && run.LeaseExpiresAt.After(leaseNow) {
			return &reviewworkflow.LeaseConflict{Cause: reviewworkflow.ErrRunLeased, Until: *run.LeaseExpiresAt}
		}
		registered, blockedUntil, err := registerLatestRun(transaction, run, leaseNow)
		if err != nil {
			return err
		}
		if !blockedUntil.IsZero() {
			publicationLeaseUntil = blockedUntil
			leaseToken = ""
			lease.Token = ""
			return nil
		}
		if !registered {
			superseded = true
			leaseToken = ""
			lease.Token = ""
			return nil
		}
		if err := requireProgressCommentOwnership(transaction, run, leaseNow); errors.Is(err, reviewworkflow.ErrRunSuperseded) {
			return reviewworkflow.ErrRunSuperseded
		} else if err != nil {
			return err
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
		if status != reviewworkflow.RunStatusPublishing {
			if err := activateProgressCommentRefreshForUpdate(transaction, run, leaseNow); err != nil {
				return err
			}
		}
		lease.ExternalCalls = run.ExternalCalls
		if lease.ExternalCalls < 0 {
			lease.ExternalCalls = 0
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, reviewworkflow.ErrRunSuperseded) {
			return reviewworkflow.RunLease{}, reviewworkflow.ErrRunSuperseded
		}
		return reviewworkflow.RunLease{}, fmt.Errorf("리뷰 실행 lease를 얻지 못했습니다: %w", err)
	}
	if superseded {
		return reviewworkflow.RunLease{}, reviewworkflow.ErrRunSuperseded
	}
	if !publicationLeaseUntil.IsZero() {
		return reviewworkflow.RunLease{}, &reviewworkflow.LeaseConflict{Cause: reviewworkflow.ErrPublicationLeased, Until: publicationLeaseUntil}
	}
	return lease, nil
}

func (r *ReviewRunStore) ResumeRun(ctx context.Context, runID uint64, leaseToken string, resumedAt time.Time) error {
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := requireRunLease(transaction, runID, leaseToken); err != nil {
			return err
		}
		var run model.ReviewRun
		if err := transaction.First(&run, runID).Error; err != nil {
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
		if err := transaction.Model(&model.PullRequestState{}).
			Where("publishing_run_id = ? AND publishing_lease_token = ?", runID, leaseToken).
			UpdateColumns(publicationClaimClearValues()).Error; err != nil {
			return err
		}
		currentAt, err := databaseTime(transaction)
		if err != nil {
			return err
		}
		return activateProgressCommentRefreshForUpdate(transaction, run, currentAt)
	})
	if err != nil {
		return fmt.Errorf("게시 조정 실행을 리뷰 상태로 되돌리지 못했습니다: %w", err)
	}
	return nil
}

func (r *ReviewRunStore) RenewRun(ctx context.Context, runID uint64, leaseToken string, heartbeatAt time.Time, leaseExpiresAt time.Time) error {
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var run model.ReviewRun
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, runID).Error; err != nil {
			return err
		}
		if err := validateRunLease(transaction, run, leaseToken); err != nil {
			return err
		}
		state, err := requireLatestRun(transaction, run)
		if err != nil {
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
		if reviewworkflow.RunStatus(run.Status) != reviewworkflow.RunStatusPublishing {
			return nil
		}
		publicationOwned := state.PublishingRunID == run.ID && state.PublishingHeadSHA == run.HeadSHA && state.PublishingLeaseToken == leaseToken
		if !publicationOwned {
			return reviewworkflow.ErrPublicationLeased
		}
		publicationUpdated := transaction.Model(&model.PullRequestState{}).
			Where("id = ? AND publishing_run_id = ? AND publishing_head_sha = ? AND publishing_lease_token = ?", state.ID, run.ID, run.HeadSHA, leaseToken).
			UpdateColumn("publishing_lease_expires_at", leaseExpiresAt)
		if publicationUpdated.Error != nil {
			return publicationUpdated.Error
		}
		if publicationUpdated.RowsAffected != 1 {
			return reviewworkflow.ErrPublicationLeased
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("리뷰 실행 lease를 갱신하지 못했습니다: %w", err)
	}
	return nil
}

func (r *ReviewRunStore) ReleaseRun(ctx context.Context, runID uint64, leaseToken string, releasedAt time.Time) error {
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
		released := transaction.Model(&model.ReviewRun{}).
			Where("id = ? AND lease_token = ? AND status NOT IN ?", runID, leaseToken, terminalRunStatuses()).
			Updates(updates)
		if released.Error != nil {
			return released.Error
		}
		if released.RowsAffected != 1 {
			return nil
		}
		if err := transaction.Model(&model.ReviewUnit{}).
			Where("review_run_id = ? AND status = ? AND lease_token <> ?", runID, string(reviewworkflow.UnitStatusRunning), "").
			Updates(map[string]any{
				"lease_token":      "",
				"lease_expires_at": nil,
			}).Error; err != nil {
			return err
		}
		if reviewworkflow.RunStatus(run.Status) != reviewworkflow.RunStatusPublishing {
			return activateProgressCommentRefreshForUpdate(transaction, run, leaseNow)
		}
		if !leaseWasActive {
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
