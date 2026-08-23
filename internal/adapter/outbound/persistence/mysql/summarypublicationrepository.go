package mysql

import (
	"context"
	"fmt"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *ReviewWorkflowRepository) ClaimSummaryPublication(ctx context.Context, target pullrequest.Target, operationKey string, orderKey string, observedAt time.Time, claimedAt time.Time, leaseExpiresAt time.Time, expiresAt time.Time) (publication.Claim, error) {
	if target.Owner == "" || target.Repository == "" || target.Number <= 0 || operationKey == "" || claimedAt.IsZero() || !leaseExpiresAt.After(claimedAt) || !expiresAt.After(claimedAt) {
		return publication.Claim{}, fmt.Errorf("요약 게시 claim 입력이 올바르지 않습니다")
	}
	if observedAt.IsZero() {
		observedAt = claimedAt
	}
	observedAt = observedAt.Truncate(time.Millisecond)
	if orderKey == "" {
		orderKey = operationKey
	}
	leaseToken, err := newLeaseToken()
	if err != nil {
		return publication.Claim{}, fmt.Errorf("요약 게시 lease를 만들지 못했습니다: %w", err)
	}
	claim := publication.Claim{LeaseToken: leaseToken}
	leased := false
	err = r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		state := model.PullRequestState{
			Owner:      target.Owner,
			Repository: target.Repository,
			Number:     target.Number,
			UpdatedAt:  claimedAt,
		}
		if err := transaction.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
			return err
		}
		state = model.PullRequestState{}
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("owner = ? AND repository = ? AND number = ?", target.Owner, target.Repository, target.Number).First(&state).Error; err != nil {
			return err
		}
		if state.SummaryCompletedKey == operationKey {
			claim = publication.Claim{Completed: true}
			return nil
		}
		if summaryPublicationIsNewer(state, operationKey, orderKey, observedAt) {
			claim = publication.Claim{Superseded: true}
			return nil
		}
		active := state.SummaryPublishingKey != "" && state.SummaryLeaseExpiresAt != nil && state.SummaryLeaseExpiresAt.After(claimedAt)
		if active {
			if !summaryPublicationIsSame(state, operationKey, orderKey, observedAt) {
				updated := transaction.Model(&model.PullRequestState{}).Where("id = ?", state.ID).Updates(map[string]any{
					"summary_operation_key": operationKey,
					"summary_order_key":     orderKey,
					"summary_observed_at":   observedAt,
					"summary_expires_at":    expiresAt,
					"updated_at":            claimedAt,
				})
				if updated.Error != nil {
					return updated.Error
				}
				if updated.RowsAffected != 1 {
					return publication.ErrLeaseLost
				}
			}
			leased = true
			claim = publication.Claim{}
			return nil
		}
		updated := transaction.Model(&model.PullRequestState{}).Where("id = ?", state.ID).Updates(map[string]any{
			"summary_operation_key":    operationKey,
			"summary_order_key":        orderKey,
			"summary_observed_at":      observedAt,
			"summary_publishing_key":   operationKey,
			"summary_lease_token":      leaseToken,
			"summary_lease_expires_at": leaseExpiresAt,
			"summary_expires_at":       expiresAt,
			"updated_at":               claimedAt,
		})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return publication.ErrLeaseLost
		}
		return nil
	})
	if err != nil {
		return publication.Claim{}, fmt.Errorf("요약 게시 권한을 얻지 못했습니다: %w", err)
	}
	if leased {
		return publication.Claim{}, fmt.Errorf("요약 게시 권한을 얻지 못했습니다: %w", publication.ErrLeased)
	}
	return claim, nil
}

func (r *ReviewWorkflowRepository) RenewSummaryPublication(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string, leaseExpiresAt time.Time) error {
	updated := r.database.WithContext(ctx).Model(&model.PullRequestState{}).
		Where("owner = ? AND repository = ? AND number = ?", target.Owner, target.Repository, target.Number).
		Where("summary_operation_key = ? AND summary_publishing_key = ? AND summary_lease_token = ?", operationKey, operationKey, leaseToken).
		UpdateColumn("summary_lease_expires_at", leaseExpiresAt)
	if updated.Error != nil {
		return fmt.Errorf("요약 게시 lease를 갱신하지 못했습니다: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return publication.ErrLeaseLost
	}
	return nil
}

func (r *ReviewWorkflowRepository) CompleteSummaryPublication(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string, completedAt time.Time, expiresAt time.Time) error {
	updated := r.database.WithContext(ctx).Model(&model.PullRequestState{}).
		Where("owner = ? AND repository = ? AND number = ?", target.Owner, target.Repository, target.Number).
		Where("summary_operation_key = ? AND summary_publishing_key = ? AND summary_lease_token = ?", operationKey, operationKey, leaseToken).
		Updates(map[string]any{
			"summary_completed_key":    operationKey,
			"summary_completed_at":     completedAt,
			"summary_expires_at":       expiresAt,
			"summary_publishing_key":   "",
			"summary_lease_token":      "",
			"summary_lease_expires_at": nil,
			"updated_at":               completedAt,
		})
	if updated.Error != nil {
		return fmt.Errorf("요약 게시 완료를 저장하지 못했습니다: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return publication.ErrLeaseLost
	}
	return nil
}

func (r *ReviewWorkflowRepository) ReleaseSummaryPublication(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string) error {
	if operationKey == "" || leaseToken == "" {
		return nil
	}
	updated := r.database.WithContext(ctx).Model(&model.PullRequestState{}).
		Where("owner = ? AND repository = ? AND number = ?", target.Owner, target.Repository, target.Number).
		Where("summary_publishing_key = ? AND summary_lease_token = ?", operationKey, leaseToken).
		Updates(map[string]any{
			"summary_publishing_key":   "",
			"summary_lease_token":      "",
			"summary_lease_expires_at": nil,
		})
	if updated.Error != nil {
		return fmt.Errorf("요약 게시 lease를 해제하지 못했습니다: %w", updated.Error)
	}
	return nil
}

func summaryPublicationIsNewer(state model.PullRequestState, operationKey string, orderKey string, observedAt time.Time) bool {
	if state.SummaryObservedAt == nil {
		return false
	}
	if state.SummaryObservedAt.After(observedAt) {
		return true
	}
	if state.SummaryObservedAt.Before(observedAt) {
		return false
	}
	if state.SummaryOrderKey != orderKey {
		return state.SummaryOrderKey > orderKey
	}
	return state.SummaryOperationKey > operationKey
}

func summaryPublicationIsSame(state model.PullRequestState, operationKey string, orderKey string, observedAt time.Time) bool {
	return state.SummaryObservedAt != nil && state.SummaryObservedAt.Equal(observedAt) && state.SummaryOrderKey == orderKey && state.SummaryOperationKey == operationKey
}
