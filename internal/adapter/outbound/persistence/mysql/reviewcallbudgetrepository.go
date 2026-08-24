package mysql

import (
	"context"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"gorm.io/gorm"
)

func (r *ReviewWorkflowRepository) ReserveExternalCall(ctx context.Context, runID uint64, runLeaseToken string, limit int) (bool, error) {
	if limit < 1 {
		return false, nil
	}
	reserved := false
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := requireRunLease(transaction, runID, runLeaseToken); err != nil {
			return err
		}
		updated := transaction.Model(&model.ReviewRun{}).
			Where("id = ? AND lease_token = ? AND COALESCE(external_calls, 0) < ?", runID, runLeaseToken, limit).
			UpdateColumn("external_calls", gorm.Expr("COALESCE(external_calls, 0) + 1"))
		if updated.Error != nil {
			return updated.Error
		}
		reserved = updated.RowsAffected == 1
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("리뷰 실행 외부 호출 예산을 예약하지 못했습니다: %w", err)
	}
	return reserved, nil
}
