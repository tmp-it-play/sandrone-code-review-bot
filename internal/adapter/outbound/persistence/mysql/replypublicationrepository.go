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

func (r *ReviewWorkflowRepository) ClaimReplyPublication(ctx context.Context, target pullrequest.Target, operationKey string, commentID int64, inThread bool, claimedAt time.Time, leaseExpiresAt time.Time, expiresAt time.Time) (publication.Claim, error) {
	if target.Owner == "" || target.Repository == "" || target.Number <= 0 || operationKey == "" || commentID <= 0 || claimedAt.IsZero() || !leaseExpiresAt.After(claimedAt) || !expiresAt.After(claimedAt) {
		return publication.Claim{}, fmt.Errorf("답글 게시 claim 입력이 올바르지 않습니다")
	}
	leaseToken, err := newLeaseToken()
	if err != nil {
		return publication.Claim{}, fmt.Errorf("답글 게시 lease를 만들지 못했습니다: %w", err)
	}
	claim := publication.Claim{LeaseToken: leaseToken}
	err = r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		entry := model.ReplyPublication{
			OperationKey: operationKey,
			Owner:        target.Owner,
			Repository:   target.Repository,
			Number:       target.Number,
			CommentID:    commentID,
			InThread:     inThread,
			ExpiresAt:    expiresAt,
			CreatedAt:    claimedAt,
			UpdatedAt:    claimedAt,
		}
		if err := transaction.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry).Error; err != nil {
			return err
		}
		entry = model.ReplyPublication{}
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_key = ?", operationKey).First(&entry).Error; err != nil {
			return err
		}
		if entry.Owner != target.Owner || entry.Repository != target.Repository || entry.Number != target.Number || entry.CommentID != commentID || entry.InThread != inThread {
			return fmt.Errorf("답글 게시 operation이 다른 대상에 연결되어 있습니다")
		}
		if entry.CompletedAt != nil && entry.ExpiresAt.After(claimedAt) {
			claim = publication.Claim{Completed: true}
			return nil
		}
		active := entry.LeaseToken != "" && entry.LeaseExpiresAt != nil && entry.LeaseExpiresAt.After(claimedAt)
		if active {
			return publication.ErrLeased
		}
		updated := transaction.Model(&model.ReplyPublication{}).Where("id = ?", entry.ID).Updates(map[string]any{
			"lease_token":      leaseToken,
			"lease_expires_at": leaseExpiresAt,
			"completed_at":     nil,
			"expires_at":       expiresAt,
			"updated_at":       claimedAt,
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
		return publication.Claim{}, fmt.Errorf("답글 게시 권한을 얻지 못했습니다: %w", err)
	}
	return claim, nil
}

func (r *ReviewWorkflowRepository) RenewReplyPublication(ctx context.Context, operationKey string, leaseToken string, leaseExpiresAt time.Time) error {
	updated := r.database.WithContext(ctx).Model(&model.ReplyPublication{}).
		Where("operation_key = ? AND lease_token = ? AND completed_at IS NULL", operationKey, leaseToken).
		UpdateColumn("lease_expires_at", leaseExpiresAt)
	if updated.Error != nil {
		return fmt.Errorf("답글 게시 lease를 갱신하지 못했습니다: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return publication.ErrLeaseLost
	}
	return nil
}

func (r *ReviewWorkflowRepository) CompleteReplyPublication(ctx context.Context, operationKey string, leaseToken string, completedAt time.Time, expiresAt time.Time) error {
	updated := r.database.WithContext(ctx).Model(&model.ReplyPublication{}).
		Where("operation_key = ? AND lease_token = ? AND completed_at IS NULL", operationKey, leaseToken).
		Updates(map[string]any{
			"lease_token":      "",
			"lease_expires_at": nil,
			"completed_at":     completedAt,
			"expires_at":       expiresAt,
			"updated_at":       completedAt,
		})
	if updated.Error != nil {
		return fmt.Errorf("답글 게시 완료를 저장하지 못했습니다: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return publication.ErrLeaseLost
	}
	return nil
}

func (r *ReviewWorkflowRepository) ReleaseReplyPublication(ctx context.Context, operationKey string, leaseToken string) error {
	if operationKey == "" || leaseToken == "" {
		return nil
	}
	updated := r.database.WithContext(ctx).Model(&model.ReplyPublication{}).
		Where("operation_key = ? AND lease_token = ? AND completed_at IS NULL", operationKey, leaseToken).
		UpdateColumns(map[string]any{
			"lease_token":      "",
			"lease_expires_at": nil,
		})
	if updated.Error != nil {
		return fmt.Errorf("답글 게시 lease를 해제하지 못했습니다: %w", updated.Error)
	}
	return nil
}
