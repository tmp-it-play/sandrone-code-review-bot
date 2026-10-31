package mysql

import (
	"encoding/json"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
)

func finishReviewedPrefix(tx *gorm.DB, child *model.ReviewUnit, result reviewworkflow.UnitResult) error {
	data, err := json.Marshal(result.Review)
	if err != nil {
		return err
	}
	child.Status = string(reviewworkflow.UnitStatusSucceeded)
	child.InputHash = result.InputHash
	child.Provider = result.Provider
	child.Model = result.Model
	child.MultipleModels = result.MultipleModels
	child.PromptTokens = result.Usage.PromptTokens
	child.CompletionTokens = result.Usage.CompletionTokens
	child.TotalTokens = result.Usage.TotalTokens
	child.ToolExecutions = result.ToolExecutions
	child.ResultJSON = string(data)
	child.FinishedAt = &result.FinishedAt
	child.HeartbeatAt = &result.FinishedAt
	if err := tx.Save(child).Error; err != nil {
		return err
	}
	return tx.Model(&model.CoverageItem{}).Where("review_run_id = ? AND review_unit_id = ? AND status = ?", child.ReviewRunID, child.ID, string(reviewworkflow.CoverageStatusPlanned)).
		Updates(map[string]any{"status": string(reviewworkflow.CoverageStatusReviewed), "reviewed_at": result.FinishedAt}).Error
}
