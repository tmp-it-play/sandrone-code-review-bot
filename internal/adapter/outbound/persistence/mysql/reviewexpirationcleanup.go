package mysql

import (
	"context"
	"fmt"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
)

func (r *ReviewRetentionRepository) DeleteExpired(ctx context.Context, now time.Time, legacyCutoff time.Time, limit int) (reviewworkflow.CleanupResult, error) {
	if limit <= 0 {
		limit = 500
	}
	cleaned := reviewworkflow.CleanupResult{}
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var occurrenceIDs []uint64
		if err := transaction.Model(&model.FindingOccurrence{}).
			Where("expires_at <= CURRENT_TIMESTAMP(6)").
			Order("id ASC").Limit(limit).Pluck("id", &occurrenceIDs).Error; err != nil {
			return err
		}
		if len(occurrenceIDs) > 0 {
			deletedOccurrences := transaction.Where("id IN ? AND expires_at <= CURRENT_TIMESTAMP(6)", occurrenceIDs).Delete(&model.FindingOccurrence{})
			if deletedOccurrences.Error != nil {
				return deletedOccurrences.Error
			}
			cleaned.Occurrences += deletedOccurrences.RowsAffected
		}

		deletedReviewPublications, err := deleteExpiredReviewPublications(transaction, now, limit)
		if err != nil {
			return err
		}
		cleaned.Publications += deletedReviewPublications

		var invalidationIDs []uint64
		if err := transaction.Model(&model.PublicationInvalidation{}).
			Where("expires_at <= ?", now).
			Order("id ASC").Limit(limit).Pluck("id", &invalidationIDs).Error; err != nil {
			return err
		}
		if len(invalidationIDs) > 0 {
			deletedInvalidations := transaction.Where("id IN ? AND expires_at <= ?", invalidationIDs, now).Delete(&model.PublicationInvalidation{})
			if deletedInvalidations.Error != nil {
				return deletedInvalidations.Error
			}
			cleaned.Publications += deletedInvalidations.RowsAffected
		}

		var replyPublicationIDs []uint64
		if err := transaction.Model(&model.ReplyPublication{}).
			Where("expires_at <= ?", now).
			Where("completed_at IS NOT NULL OR lease_token = '' OR lease_expires_at IS NULL OR lease_expires_at <= ?", now).
			Order("id ASC").Limit(limit).Pluck("id", &replyPublicationIDs).Error; err != nil {
			return err
		}
		if len(replyPublicationIDs) > 0 {
			deletedPublications := transaction.Where("id IN ? AND expires_at <= ?", replyPublicationIDs, now).
				Where("completed_at IS NOT NULL OR lease_token = '' OR lease_expires_at IS NULL OR lease_expires_at <= ?", now).
				Delete(&model.ReplyPublication{})
			if deletedPublications.Error != nil {
				return deletedPublications.Error
			}
			cleaned.Publications += deletedPublications.RowsAffected
		}

		var runIDs []uint64
		if err := transaction.Model(&model.ReviewRun{}).Where("expires_at IS NOT NULL AND expires_at <= ?", now).Order("id ASC").Limit(limit).Pluck("id", &runIDs).Error; err != nil {
			return err
		}
		if len(runIDs) > 0 {
			var linkedReviewIDs []uint64
			if err := transaction.Model(&model.Review{}).Where("review_run_id IN ?", runIDs).Pluck("id", &linkedReviewIDs).Error; err != nil {
				return err
			}
			if len(linkedReviewIDs) > 0 {
				deletedFindings := transaction.Where("review_id IN ?", linkedReviewIDs).Delete(&model.Finding{})
				if deletedFindings.Error != nil {
					return deletedFindings.Error
				}
				cleaned.Findings += deletedFindings.RowsAffected
				deletedReviews := transaction.Where("id IN ?", linkedReviewIDs).Delete(&model.Review{})
				if deletedReviews.Error != nil {
					return deletedReviews.Error
				}
				cleaned.Reviews += deletedReviews.RowsAffected
			}
			if err := transaction.Where("review_run_id IN ?", runIDs).Delete(&model.CoverageItem{}).Error; err != nil {
				return err
			}
			if err := transaction.Where("review_run_id IN ?", runIDs).Delete(&model.ReviewUnit{}).Error; err != nil {
				return err
			}
			if err := transaction.Where("review_run_id IN ?", runIDs).Delete(&model.ProgressCommentOwnership{}).Error; err != nil {
				return err
			}
			deleted := transaction.Where("id IN ?", runIDs).Delete(&model.ReviewRun{})
			if deleted.Error != nil {
				return deleted.Error
			}
			cleaned.Runs = deleted.RowsAffected
		}

		var reviewIDs []uint64
		if err := transaction.Model(&model.Review{}).Where("review_run_id IS NULL AND finished_at <= ?", legacyCutoff).Order("id ASC").Limit(limit).Pluck("id", &reviewIDs).Error; err != nil {
			return err
		}
		if len(reviewIDs) > 0 {
			deletedFindings := transaction.Where("review_id IN ?", reviewIDs).Delete(&model.Finding{})
			if deletedFindings.Error != nil {
				return deletedFindings.Error
			}
			cleaned.Findings += deletedFindings.RowsAffected
			deletedReviews := transaction.Where("id IN ?", reviewIDs).Delete(&model.Review{})
			if deletedReviews.Error != nil {
				return deletedReviews.Error
			}
			cleaned.Reviews += deletedReviews.RowsAffected
		}
		var orphanFindingIDs []uint64
		orphanFindingCondition := "created_at <= ? AND (review_id = 0 OR NOT EXISTS (SELECT 1 FROM reviews WHERE reviews.id = findings.review_id))"
		if err := transaction.Model(&model.Finding{}).Where(orphanFindingCondition, legacyCutoff).Order("id ASC").Limit(limit).Pluck("id", &orphanFindingIDs).Error; err != nil {
			return err
		}
		if len(orphanFindingIDs) > 0 {
			deletedOrphans := transaction.Where("id IN ?", orphanFindingIDs).Where(orphanFindingCondition, legacyCutoff).Delete(&model.Finding{})
			if deletedOrphans.Error != nil {
				return deletedOrphans.Error
			}
			cleaned.Findings += deletedOrphans.RowsAffected
		}

		var expiredClaimStateIDs []uint64
		if err := transaction.Model(&model.PullRequestState{}).
			Where("COALESCE(publishing_run_id, 0) <> 0 AND (publishing_lease_expires_at IS NULL OR publishing_lease_expires_at <= ?)", now).
			Order("id ASC").Limit(limit).Pluck("id", &expiredClaimStateIDs).Error; err != nil {
			return err
		}
		if len(expiredClaimStateIDs) > 0 {
			clearedClaims := transaction.Model(&model.PullRequestState{}).
				Where("id IN ? AND COALESCE(publishing_run_id, 0) <> 0 AND (publishing_lease_expires_at IS NULL OR publishing_lease_expires_at <= ?)", expiredClaimStateIDs, now).
				UpdateColumns(publicationClaimClearValues())
			if clearedClaims.Error != nil {
				return clearedClaims.Error
			}
			cleaned.States += clearedClaims.RowsAffected
		}

		var expiredSummaryStateIDs []uint64
		if err := transaction.Model(&model.PullRequestState{}).
			Where("summary_operation_key <> '' AND summary_expires_at IS NOT NULL AND summary_expires_at <= ?", now).
			Where("summary_publishing_key = '' OR summary_lease_expires_at IS NULL OR summary_lease_expires_at <= ?", now).
			Order("id ASC").Limit(limit).Pluck("id", &expiredSummaryStateIDs).Error; err != nil {
			return err
		}
		if len(expiredSummaryStateIDs) > 0 {
			clearedSummaries := transaction.Model(&model.PullRequestState{}).
				Where("id IN ? AND summary_operation_key <> '' AND summary_expires_at IS NOT NULL AND summary_expires_at <= ?", expiredSummaryStateIDs, now).
				Where("summary_publishing_key = '' OR summary_lease_expires_at IS NULL OR summary_lease_expires_at <= ?", now).
				UpdateColumns(summaryPublicationClearValues())
			if clearedSummaries.Error != nil {
				return clearedSummaries.Error
			}
			cleaned.States += clearedSummaries.RowsAffected
		}

		watermarkStateIDs, err := expiredWatermarkStateIDs(transaction, legacyCutoff, limit)
		if err != nil {
			return err
		}
		if len(watermarkStateIDs) > 0 {
			clearedStates := transaction.Model(&model.PullRequestState{}).
				Where("id IN ? AND last_reviewed_sha <> '' AND (last_reviewed_at IS NULL OR last_reviewed_at <= ?)", watermarkStateIDs, legacyCutoff).
				UpdateColumns(map[string]any{
					"last_reviewed_sha":    "",
					"last_reviewed_run_id": 0,
					"last_reviewed_at":     nil,
				})
			if clearedStates.Error != nil {
				return clearedStates.Error
			}
			cleaned.States += clearedStates.RowsAffected
		}
		var latestStateIDs []uint64
		if err := transaction.Model(&model.PullRequestState{}).
			Where("COALESCE(latest_run_id, 0) <> 0 AND latest_run_at IS NOT NULL AND latest_run_at <= ?", legacyCutoff).
			Order("id ASC").Limit(limit).Pluck("id", &latestStateIDs).Error; err != nil {
			return err
		}
		if len(latestStateIDs) > 0 {
			clearedLatest := transaction.Model(&model.PullRequestState{}).
				Where("id IN ? AND COALESCE(latest_run_id, 0) <> 0 AND latest_run_at IS NOT NULL AND latest_run_at <= ?", latestStateIDs, legacyCutoff).
				UpdateColumns(map[string]any{
					"latest_run_id":      0,
					"latest_head_sha":    "",
					"latest_observed_at": nil,
					"latest_order_key":   "",
					"latest_run_at":      nil,
				})
			if clearedLatest.Error != nil {
				return clearedLatest.Error
			}
			cleaned.States += clearedLatest.RowsAffected
		}

		var deletableStateIDs []uint64
		if err := transaction.Model(&model.PullRequestState{}).
			Where("last_reviewed_sha = '' AND COALESCE(latest_run_id, 0) = 0 AND COALESCE(publishing_run_id, 0) = 0 AND summary_operation_key = '' AND summary_publishing_key = '' AND updated_at <= ?", legacyCutoff).
			Order("id ASC").Limit(limit).Pluck("id", &deletableStateIDs).Error; err != nil {
			return err
		}
		if len(deletableStateIDs) > 0 {
			deletedStates := transaction.
				Where("id IN ? AND last_reviewed_sha = '' AND COALESCE(latest_run_id, 0) = 0 AND COALESCE(publishing_run_id, 0) = 0 AND summary_operation_key = '' AND summary_publishing_key = '' AND updated_at <= ?", deletableStateIDs, legacyCutoff).
				Delete(&model.PullRequestState{})
			if deletedStates.Error != nil {
				return deletedStates.Error
			}
			cleaned.States += deletedStates.RowsAffected
		}
		return nil
	})
	if err != nil {
		return reviewworkflow.CleanupResult{}, fmt.Errorf("만료된 리뷰 데이터를 제거하지 못했습니다: %w", err)
	}
	return cleaned, nil
}

func summaryPublicationClearValues() map[string]any {
	values := map[string]any{
		"summary_operation_key":    "",
		"summary_order_key":        "",
		"summary_observed_at":      nil,
		"summary_publishing_key":   "",
		"summary_lease_token":      "",
		"summary_lease_expires_at": nil,
		"summary_completed_key":    "",
		"summary_completed_at":     nil,
		"summary_expires_at":       nil,
		"summary_external_calls":   0,
	}
	mergeValues(values, summaryCompletionResetValues())
	return values
}

func expiredWatermarkStateIDs(transaction *gorm.DB, cutoff time.Time, limit int) ([]uint64, error) {
	stateIDs := make([]uint64, 0, limit)
	if err := transaction.Model(&model.PullRequestState{}).
		Where("last_reviewed_sha <> '' AND last_reviewed_at IS NOT NULL AND last_reviewed_at <= ?", cutoff).
		Order("id ASC").Limit(limit).Pluck("id", &stateIDs).Error; err != nil {
		return nil, err
	}
	remaining := limit - len(stateIDs)
	if remaining <= 0 {
		return stateIDs, nil
	}
	legacyStateIDs := make([]uint64, 0, remaining)
	if err := transaction.Model(&model.PullRequestState{}).
		Where("last_reviewed_sha <> '' AND last_reviewed_at IS NULL").
		Order("id ASC").Limit(remaining).Pluck("id", &legacyStateIDs).Error; err != nil {
		return nil, err
	}
	return append(stateIDs, legacyStateIDs...), nil
}
