package mysql

import (
	"context"
	"errors"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
)

func (r *ReviewRunStore) FinishDetachedProgressFailure(ctx context.Context, run reviewworkflow.Run, result reviewworkflow.RunResult) (uint64, reviewworkflow.RunStatus, error) {
	entry := model.ReviewRun{}
	status := result.Status
	result.DetachedFinalization = true
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		entry = model.ReviewRun{}
		status = result.Status
		err = r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
			var createErr error
			entry, createErr = createOrGetReviewRun(transaction, run)
			if createErr != nil {
				return createErr
			}
			return finishReviewRun(transaction, entry.ID, result, &status)
		})
		if !errors.Is(err, errProgressCommentOwnershipRace) {
			break
		}
	}
	if err != nil {
		return 0, "", fmt.Errorf("분리된 진행 코멘트 실패를 저장하지 못했습니다: %w", err)
	}
	return entry.ID, status, nil
}
