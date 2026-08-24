package mysql

import (
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
)

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
