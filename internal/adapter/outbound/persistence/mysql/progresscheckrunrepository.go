package mysql

import (
	"context"
	"errors"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"gorm.io/gorm"
)

func (r *ReviewRunStore) ProgressCheckRun(ctx context.Context, marker string) (int64, bool, error) {
	var entry model.ProgressCommentOwnership
	err := r.database.WithContext(ctx).Select("check_run_id").Where("marker = ?", marker).First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("진행 체크를 조회하지 못했습니다: %w", err)
	}
	return entry.CheckRunID, entry.CheckRunID > 0, nil
}

func (r *ReviewRunStore) SaveProgressCheckRun(ctx context.Context, marker string, checkRunID int64) error {
	if err := r.database.WithContext(ctx).Model(&model.ProgressCommentOwnership{}).Where("marker = ?", marker).Update("check_run_id", checkRunID).Error; err != nil {
		return fmt.Errorf("진행 체크를 저장하지 못했습니다: %w", err)
	}
	return nil
}
