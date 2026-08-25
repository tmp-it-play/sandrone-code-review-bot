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

func (r *ReviewPublicationStore) ClaimPublication(ctx context.Context, runID uint64, runLeaseToken string, claimedAt time.Time, leaseExpiresAt time.Time) error {
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
			return &reviewworkflow.LeaseConflict{Cause: reviewworkflow.ErrPublicationLeased, Until: *state.PublishingLeaseExpiresAt}
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
