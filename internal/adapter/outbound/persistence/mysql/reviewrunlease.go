package mysql

import (
	"context"
	"fmt"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const publicationRetryDelay = 5 * time.Minute

func (r *ReviewRunStore) AcquireRun(ctx context.Context, runID uint64, startedAt time.Time, leaseExpiresAt time.Time) (string, error) {
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

func (r *ReviewRunStore) ResumeRun(ctx context.Context, runID uint64, leaseToken string, resumedAt time.Time) error {
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

func (r *ReviewRunStore) RenewRun(ctx context.Context, runID uint64, leaseToken string, heartbeatAt time.Time, leaseExpiresAt time.Time) error {
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
