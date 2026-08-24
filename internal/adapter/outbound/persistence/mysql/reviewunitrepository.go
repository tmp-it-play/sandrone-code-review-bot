package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *ReviewExecutionStore) StartUnit(ctx context.Context, runID uint64, runLeaseToken string, unitHash string, inputHash string, startedAt time.Time, leaseExpiresAt time.Time) (reviewworkflow.UnitClaim, error) {
	leaseToken, err := newLeaseToken()
	if err != nil {
		return reviewworkflow.UnitClaim{}, fmt.Errorf("unit lease를 만들지 못했습니다: %w", err)
	}
	claim := reviewworkflow.UnitClaim{LeaseToken: leaseToken}
	err = r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := requireRunLease(transaction, runID, runLeaseToken); err != nil {
			return err
		}
		leaseNow, err := databaseTime(transaction)
		if err != nil {
			return err
		}
		if err := transaction.Model(&model.ReviewRun{}).Where("id = ? AND lease_token = ?", runID, runLeaseToken).Updates(map[string]any{
			"heartbeat_at":     startedAt,
			"lease_expires_at": leaseExpiresAt,
		}).Error; err != nil {
			return err
		}
		var unit model.ReviewUnit
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("review_run_id = ? AND unit_hash = ?", runID, unitHash).First(&unit).Error; err != nil {
			return err
		}
		if reviewworkflow.UnitStatus(unit.Status) == reviewworkflow.UnitStatusSucceeded {
			if inputHash != "" && unit.InputHash == inputHash && unit.ResultJSON != "" {
				stored, err := storedUnitResult(unit)
				if err == nil {
					claim = reviewworkflow.UnitClaim{Completed: true, Reused: unit.Reused, Result: stored}
					return nil
				}
			}
			if err := transaction.Model(&model.ReviewUnit{}).Where("id = ? AND status = ?", unit.ID, string(reviewworkflow.UnitStatusSucceeded)).Updates(map[string]any{
				"status":           string(reviewworkflow.UnitStatusFailed),
				"result_json":      "",
				"reused":           false,
				"error_summary":    "input_changed",
				"lease_token":      "",
				"lease_expires_at": nil,
			}).Error; err != nil {
				return err
			}
			if err := transaction.Model(&model.CoverageItem{}).
				Where("review_run_id = ? AND review_unit_id = ? AND status IN ?", runID, unit.ID, []string{string(reviewworkflow.CoverageStatusReviewed), string(reviewworkflow.CoverageStatusFailed)}).
				Updates(map[string]any{"status": string(reviewworkflow.CoverageStatusPlanned), "reviewed_at": nil}).Error; err != nil {
				return err
			}
			unit.Status = string(reviewworkflow.UnitStatusFailed)
		}
		claim.Result = reviewworkflow.UnitResult{
			Provider:       unit.Provider,
			Model:          unit.Model,
			MultipleModels: unit.MultipleModels,
			Usage:          llm.Usage{PromptTokens: unit.PromptTokens, CompletionTokens: unit.CompletionTokens, TotalTokens: unit.TotalTokens},
			ToolExecutions: unit.ToolExecutions,
		}
		updates := map[string]any{
			"status":           string(reviewworkflow.UnitStatusRunning),
			"input_hash":       inputHash,
			"attempt_count":    gorm.Expr("attempt_count + 1"),
			"started_at":       startedAt,
			"finished_at":      nil,
			"heartbeat_at":     startedAt,
			"result_json":      "",
			"reused":           false,
			"error_summary":    "",
			"lease_token":      leaseToken,
			"lease_expires_at": leaseExpiresAt,
		}
		result := transaction.Model(&model.ReviewUnit{}).
			Where("id = ?", unit.ID).
			Where("status IN ? OR (status = ? AND (lease_expires_at IS NULL OR lease_expires_at <= ?))", []string{
				string(reviewworkflow.UnitStatusPending),
				string(reviewworkflow.UnitStatusFailed),
			}, string(reviewworkflow.UnitStatusRunning), leaseNow).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("unit을 실행 가능한 상태로 전이하지 못했습니다")
		}
		return transaction.Model(&model.CoverageItem{}).
			Where("review_run_id = ? AND review_unit_id = ? AND status = ?", runID, unit.ID, string(reviewworkflow.CoverageStatusFailed)).Updates(map[string]any{
			"status":      string(reviewworkflow.CoverageStatusPlanned),
			"reviewed_at": nil,
		}).Error
	})
	if err != nil {
		return reviewworkflow.UnitClaim{}, fmt.Errorf("리뷰 unit을 시작하지 못했습니다: %w", err)
	}
	return claim, nil
}

func (r *ReviewExecutionStore) FinishUnit(ctx context.Context, runID uint64, runLeaseToken string, unitHash string, leaseToken string, result reviewworkflow.UnitResult) error {
	resultJSON := ""
	if result.Status == reviewworkflow.UnitStatusSucceeded {
		encoded, err := json.Marshal(result.Review)
		if err != nil {
			return fmt.Errorf("리뷰 unit 결과를 직렬화하지 못했습니다: %w", err)
		}
		resultJSON = string(encoded)
	}
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := requireRunLease(transaction, runID, runLeaseToken); err != nil {
			return err
		}
		var unit model.ReviewUnit
		if err := transaction.Where("review_run_id = ? AND unit_hash = ? AND lease_token = ? AND lease_expires_at > CURRENT_TIMESTAMP(6)", runID, unitHash, leaseToken).First(&unit).Error; err != nil {
			return err
		}
		provider := result.Provider
		unitModel := result.Model
		multipleModels := unit.MultipleModels || result.MultipleModels || result.Provider == "multiple" || result.Model == "multiple"
		if result.Status != reviewworkflow.UnitStatusSucceeded {
			provider, unitModel = mergedUnitIdentity(unit.Provider, unit.Model, result.Provider, result.Model)
		}
		if unit.Provider != "" && unit.Model != "" && result.Provider != "" && result.Model != "" && (unit.Provider != result.Provider || unit.Model != result.Model) {
			multipleModels = true
		}
		updated := transaction.Model(&model.ReviewUnit{}).
			Where("id = ? AND status = ? AND lease_token = ? AND lease_expires_at > CURRENT_TIMESTAMP(6)", unit.ID, string(reviewworkflow.UnitStatusRunning), leaseToken).
			Updates(map[string]any{
				"status":            string(result.Status),
				"input_hash":        result.InputHash,
				"provider":          provider,
				"model":             unitModel,
				"multiple_models":   multipleModels,
				"prompt_tokens":     gorm.Expr("prompt_tokens + ?", result.Usage.PromptTokens),
				"completion_tokens": gorm.Expr("completion_tokens + ?", result.Usage.CompletionTokens),
				"total_tokens":      gorm.Expr("total_tokens + ?", result.Usage.TotalTokens),
				"tool_executions":   gorm.Expr("tool_executions + ?", result.ToolExecutions),
				"result_json":       resultJSON,
				"reused":            result.Reused,
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
			return errors.New("unit 완료 상태가 충돌했습니다")
		}
		coverageStatus := reviewworkflow.CoverageStatusFailed
		var reviewedAt *time.Time
		if result.Status == reviewworkflow.UnitStatusSucceeded {
			coverageStatus = reviewworkflow.CoverageStatusReviewed
			reviewedAt = &result.FinishedAt
		}
		return transaction.Model(&model.CoverageItem{}).
			Where("review_run_id = ? AND review_unit_id = ? AND status = ?", runID, unit.ID, string(reviewworkflow.CoverageStatusPlanned)).
			Updates(map[string]any{
				"status":      string(coverageStatus),
				"reviewed_at": reviewedAt,
			}).Error
	})
	if err != nil {
		return fmt.Errorf("리뷰 unit 결과를 저장하지 못했습니다: %w", err)
	}
	return nil
}

func mergedUnitIdentity(previousProvider string, previousModel string, currentProvider string, currentModel string) (string, string) {
	if currentProvider == "" && currentModel == "" {
		return previousProvider, previousModel
	}
	if previousProvider == "" && previousModel == "" {
		return currentProvider, currentModel
	}
	if previousProvider == currentProvider && previousModel == currentModel {
		return currentProvider, currentModel
	}
	return "multiple", "multiple"
}
