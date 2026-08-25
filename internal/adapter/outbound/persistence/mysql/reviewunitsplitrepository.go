package mysql

import (
	"context"
	"errors"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/mapper"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *ReviewExecutionStore) SplitUnit(ctx context.Context, runID uint64, runLeaseToken string, split reviewworkflow.UnitSplit) ([]reviewworkflow.Unit, error) {
	if err := validateUnitSplitInput(split); err != nil {
		return nil, fmt.Errorf("리뷰 unit 분할 요청이 유효하지 않습니다: %w", err)
	}
	leaves := make([]reviewworkflow.Unit, 0, 2)
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := requireRunLease(transaction, runID, runLeaseToken); err != nil {
			return err
		}
		var parent model.ReviewUnit
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("review_run_id = ? AND unit_hash = ?", runID, split.ParentHash).First(&parent).Error; err != nil {
			return err
		}
		children, err := prepareSplitChildren(parent, split.Children)
		if err != nil {
			return err
		}
		splitHash := unitSplitHash(split.ParentInputHash, children, split.Assignments)
		if reviewworkflow.UnitStatus(parent.Status) == reviewworkflow.UnitStatusSplit {
			if err := validateStoredUnitSplit(transaction, runID, parent, children, split.Assignments, splitHash, split.ParentInputHash); err != nil {
				return err
			}
			entries, err := loadDescendantLeafEntries(transaction, runID, parent.UnitHash)
			if err != nil {
				return err
			}
			leaves = mapReviewUnits(entries)
			return nil
		}
		if err := validateRunningSplitParent(transaction, parent, split); err != nil {
			return err
		}
		assignmentIDs, err := validateSplitAssignments(transaction, runID, parent, children, split.Assignments)
		if err != nil {
			return err
		}
		childEntries := make([]model.ReviewUnit, 0, len(children))
		for _, child := range children {
			child.RunID = runID
			entry := mapper.ToReviewUnitModel(child)
			if err := transaction.Create(&entry).Error; err != nil {
				return err
			}
			childEntries = append(childEntries, entry)
		}
		childIDs := make(map[string]uint64, len(childEntries))
		for _, child := range childEntries {
			childIDs[child.UnitHash] = child.ID
		}
		for childHash, coverageIDs := range assignmentIDs {
			updated := transaction.Model(&model.CoverageItem{}).
				Where("review_run_id = ? AND review_unit_id = ? AND status = ? AND id IN ?", runID, parent.ID, string(reviewworkflow.CoverageStatusPlanned), coverageIDs).
				Update("review_unit_id", childIDs[childHash])
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != int64(len(coverageIDs)) {
				return errors.New("분할할 coverage 연결이 충돌했습니다")
			}
		}
		if err := finishSplitParent(transaction, parent, split, splitHash); err != nil {
			return err
		}
		updatedRun := transaction.Model(&model.ReviewRun{}).
			Where("id = ? AND lease_token = ?", runID, runLeaseToken).
			Updates(map[string]any{
				"plan_revision": gorm.Expr("plan_revision + 1"),
				"heartbeat_at":  split.RefinedAt,
			})
		if updatedRun.Error != nil {
			return updatedRun.Error
		}
		if updatedRun.RowsAffected != 1 {
			return reviewworkflow.ErrRunLeased
		}
		leaves = mapReviewUnits(childEntries)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("리뷰 unit을 분할하지 못했습니다: %w", err)
	}
	return leaves, nil
}

func finishSplitParent(transaction *gorm.DB, parent model.ReviewUnit, split reviewworkflow.UnitSplit, splitHash string) error {
	result := split.ParentResult
	provider, unitModel := mergedUnitIdentity(parent.Provider, parent.Model, result.Provider, result.Model)
	multipleModels := parent.MultipleModels || result.MultipleModels || provider == "multiple" || unitModel == "multiple"
	updated := transaction.Model(&model.ReviewUnit{}).
		Where("id = ? AND status = ? AND lease_token = ? AND lease_expires_at > CURRENT_TIMESTAMP(6)", parent.ID, string(reviewworkflow.UnitStatusRunning), split.ParentLeaseToken).
		Updates(map[string]any{
			"status":            string(reviewworkflow.UnitStatusSplit),
			"input_hash":        split.ParentInputHash,
			"provider":          provider,
			"model":             unitModel,
			"multiple_models":   multipleModels,
			"prompt_tokens":     gorm.Expr("prompt_tokens + ?", result.Usage.PromptTokens),
			"completion_tokens": gorm.Expr("completion_tokens + ?", result.Usage.CompletionTokens),
			"total_tokens":      gorm.Expr("total_tokens + ?", result.Usage.TotalTokens),
			"tool_executions":   gorm.Expr("tool_executions + ?", result.ToolExecutions),
			"result_json":       "",
			"reused":            false,
			"retryable":         false,
			"retry_at":          nil,
			"split_hash":        splitHash,
			"error_summary":     boundedText(result.Error, 1000),
			"finished_at":       result.FinishedAt,
			"heartbeat_at":      result.FinishedAt,
			"lease_token":       "",
			"lease_expires_at":  nil,
		})
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected != 1 {
		return errors.New("분할할 unit 완료 상태가 충돌했습니다")
	}
	return nil
}
