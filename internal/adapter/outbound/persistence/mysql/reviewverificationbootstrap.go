package mysql

import (
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"gorm.io/gorm"
)

func backfillReviewVerificationCompletion(database *gorm.DB) error {
	if !database.Migrator().HasTable(&model.ReviewVerification{}) || !database.Migrator().HasColumn(&model.ReviewVerification{}, "completed") {
		return nil
	}
	result := database.Model(&model.ReviewVerification{}).
		Where("completed = ? AND input_hash <> '' AND provider <> '' AND model <> ''", false).
		Updates(map[string]any{
			"completed":               true,
			"attempt_count":           gorm.Expr("GREATEST(attempt_count, 1)"),
			"last_attempt_input_hash": gorm.Expr("input_hash"),
			"last_attempt_succeeded":  true,
			"result_completed_at":     gorm.Expr("finished_at"),
		})
	if result.Error != nil {
		return fmt.Errorf("기존 finding verifier 완료 상태를 복원하지 못했습니다: %w", result.Error)
	}
	result = database.Model(&model.ReviewVerification{}).
		Where("completed = ? AND input_hash <> '' AND (last_attempt_input_hash = '' OR result_completed_at IS NULL)", true).
		Updates(map[string]any{
			"attempt_count":           gorm.Expr("GREATEST(attempt_count, 1)"),
			"last_attempt_input_hash": gorm.Expr("input_hash"),
			"last_attempt_succeeded":  true,
			"result_completed_at":     gorm.Expr("finished_at"),
		})
	if result.Error != nil {
		return fmt.Errorf("기존 finding verifier 완료 metadata를 복원하지 못했습니다: %w", result.Error)
	}
	return nil
}
