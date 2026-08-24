package mysql

import (
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"gorm.io/gorm"
)

func deleteExpiredRunLifecycle(transaction *gorm.DB, runID uint64) error {
	if err := transaction.Model(&model.PullRequestState{}).Where("latest_run_id = ?", runID).Updates(map[string]any{
		"latest_run_id":      0,
		"latest_head_sha":    "",
		"latest_observed_at": nil,
		"latest_order_key":   "",
		"latest_run_at":      nil,
	}).Error; err != nil {
		return err
	}
	if err := transaction.Model(&model.PullRequestState{}).Where("publishing_run_id = ?", runID).UpdateColumns(publicationClaimClearValues()).Error; err != nil {
		return err
	}
	if err := transaction.Model(&model.PullRequestState{}).Where("last_reviewed_run_id = ?", runID).Updates(map[string]any{
		"last_reviewed_sha":    "",
		"last_reviewed_run_id": 0,
		"last_reviewed_at":     nil,
	}).Error; err != nil {
		return err
	}
	var reviewIDs []uint64
	if err := transaction.Model(&model.Review{}).Where("review_run_id = ?", runID).Pluck("id", &reviewIDs).Error; err != nil {
		return err
	}
	if len(reviewIDs) > 0 {
		if err := transaction.Where("source_review_id IN ? OR source_run_id = ?", reviewIDs, runID).Delete(&model.FindingOccurrence{}).Error; err != nil {
			return err
		}
		if err := transaction.Where("review_id IN ?", reviewIDs).Delete(&model.Finding{}).Error; err != nil {
			return err
		}
		if err := transaction.Where("id IN ?", reviewIDs).Delete(&model.Review{}).Error; err != nil {
			return err
		}
	} else if err := transaction.Where("source_run_id = ?", runID).Delete(&model.FindingOccurrence{}).Error; err != nil {
		return err
	}
	if err := transaction.Where("review_run_id = ?", runID).Delete(&model.PublicationInvalidation{}).Error; err != nil {
		return err
	}
	if err := transaction.Where("review_run_id = ?", runID).Delete(&model.ReviewPublication{}).Error; err != nil {
		return err
	}
	if err := transaction.Where("review_run_id = ?", runID).Delete(&model.ReviewVerification{}).Error; err != nil {
		return err
	}
	if err := transaction.Where("review_run_id = ?", runID).Delete(&model.CoverageItem{}).Error; err != nil {
		return err
	}
	if err := transaction.Where("review_run_id = ?", runID).Delete(&model.ReviewUnit{}).Error; err != nil {
		return err
	}
	return transaction.Where("id = ?", runID).Delete(&model.ReviewRun{}).Error
}
