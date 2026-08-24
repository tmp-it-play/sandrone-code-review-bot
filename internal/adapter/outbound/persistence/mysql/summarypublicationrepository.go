package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *ReviewWorkflowRepository) ClaimSummaryPublication(ctx context.Context, target pullrequest.Target, operationKey string, orderKey string, observedAt time.Time, claimedAt time.Time, leaseExpiresAt time.Time, expiresAt time.Time) (publication.Claim, error) {
	if target.Owner == "" || target.Repository == "" || target.Number <= 0 || operationKey == "" || claimedAt.IsZero() || !leaseExpiresAt.After(claimedAt) || !expiresAt.After(claimedAt) {
		return publication.Claim{}, fmt.Errorf("요약 게시 claim 입력이 올바르지 않습니다")
	}
	if observedAt.IsZero() {
		observedAt = claimedAt
	}
	observedAt = observedAt.Truncate(time.Millisecond)
	if orderKey == "" {
		orderKey = operationKey
	}
	leaseToken, err := newLeaseToken()
	if err != nil {
		return publication.Claim{}, fmt.Errorf("요약 게시 lease를 만들지 못했습니다: %w", err)
	}
	claim := publication.Claim{LeaseToken: leaseToken}
	leased := false
	err = r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		state := model.PullRequestState{
			Owner:      target.Owner,
			Repository: target.Repository,
			Number:     target.Number,
			UpdatedAt:  claimedAt,
		}
		if err := transaction.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
			return err
		}
		state = model.PullRequestState{}
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("owner = ? AND repository = ? AND number = ?", target.Owner, target.Repository, target.Number).First(&state).Error; err != nil {
			return err
		}
		claimCurrentAt, err := databaseTime(transaction)
		if err != nil {
			return err
		}
		expired := state.SummaryOperationKey != "" && (state.SummaryExpiresAt == nil || !state.SummaryExpiresAt.After(claimCurrentAt))
		if !expired && state.SummaryCompletedKey == operationKey {
			claim = publication.Claim{Completed: true}
			return nil
		}
		if !expired && summaryPublicationIsNewer(state, operationKey, orderKey, observedAt) {
			claim = publication.Claim{Superseded: true}
			return nil
		}
		active := !expired && state.SummaryPublishingKey != "" && state.SummaryLeaseExpiresAt != nil && state.SummaryLeaseExpiresAt.After(claimCurrentAt)
		sameOperation := !expired && state.SummaryOperationKey == operationKey
		lifecycleExpiresAt := expiresAt
		if sameOperation && state.SummaryExpiresAt != nil {
			lifecycleExpiresAt = *state.SummaryExpiresAt
		}
		lifecycleLeaseExpiresAt := leaseExpiresAt
		if lifecycleLeaseExpiresAt.After(lifecycleExpiresAt) {
			lifecycleLeaseExpiresAt = lifecycleExpiresAt
		}
		if !lifecycleExpiresAt.After(claimCurrentAt) || !lifecycleLeaseExpiresAt.After(claimCurrentAt) {
			return fmt.Errorf("요약 게시 lifecycle이 이미 만료되었습니다")
		}
		if active {
			if !summaryPublicationIsSame(state, operationKey, orderKey, observedAt) {
				externalCalls := state.SummaryExternalCalls
				if !sameOperation {
					externalCalls = 0
				}
				values := map[string]any{
					"summary_operation_key":  operationKey,
					"summary_order_key":      orderKey,
					"summary_observed_at":    observedAt,
					"summary_expires_at":     lifecycleExpiresAt,
					"summary_external_calls": externalCalls,
					"updated_at":             claimedAt,
				}
				if !sameOperation {
					mergeValues(values, summaryCompletionResetValues())
				}
				updated := transaction.Model(&model.PullRequestState{}).Where("id = ?", state.ID).Updates(values)
				if updated.Error != nil {
					return updated.Error
				}
				if updated.RowsAffected != 1 {
					return publication.ErrLeaseLost
				}
			}
			leased = true
			claim = publication.Claim{}
			return nil
		}
		externalCalls := state.SummaryExternalCalls
		if !sameOperation {
			externalCalls = 0
		}
		values := map[string]any{
			"summary_operation_key":    operationKey,
			"summary_order_key":        orderKey,
			"summary_observed_at":      observedAt,
			"summary_publishing_key":   operationKey,
			"summary_lease_token":      leaseToken,
			"summary_lease_expires_at": lifecycleLeaseExpiresAt,
			"summary_expires_at":       lifecycleExpiresAt,
			"summary_external_calls":   externalCalls,
			"updated_at":               claimedAt,
		}
		if !sameOperation {
			mergeValues(values, summaryCompletionResetValues())
		}
		updated := transaction.Model(&model.PullRequestState{}).Where("id = ?", state.ID).Updates(values)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return publication.ErrLeaseLost
		}
		return nil
	})
	if err != nil {
		return publication.Claim{}, fmt.Errorf("요약 게시 권한을 얻지 못했습니다: %w", err)
	}
	if leased {
		return publication.Claim{}, fmt.Errorf("요약 게시 권한을 얻지 못했습니다: %w", publication.ErrLeased)
	}
	return claim, nil
}

func mergeValues(target map[string]any, values map[string]any) {
	for key, value := range values {
		target[key] = value
	}
}

func (r *ReviewWorkflowRepository) ReserveSummaryExternalCall(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string, limit int) (bool, error) {
	if limit < 1 {
		return false, nil
	}
	updated := r.database.WithContext(ctx).Model(&model.PullRequestState{}).
		Where("owner = ? AND repository = ? AND number = ?", target.Owner, target.Repository, target.Number).
		Where("summary_operation_key = ? AND summary_publishing_key = ? AND summary_lease_token = ? AND summary_lease_expires_at > CURRENT_TIMESTAMP AND summary_expires_at > CURRENT_TIMESTAMP AND COALESCE(summary_external_calls, 0) < ?", operationKey, operationKey, leaseToken, limit).
		UpdateColumn("summary_external_calls", gorm.Expr("COALESCE(summary_external_calls, 0) + 1"))
	if updated.Error != nil {
		return false, fmt.Errorf("요약 외부 호출 예산을 예약하지 못했습니다: %w", updated.Error)
	}
	return updated.RowsAffected == 1, nil
}

func (r *ReviewWorkflowRepository) SummaryCompletion(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string, inputHash string, currentAt time.Time) (publication.CompletionCheckpoint, bool, error) {
	if operationKey == "" || leaseToken == "" || inputHash == "" || currentAt.IsZero() {
		return publication.CompletionCheckpoint{}, false, fmt.Errorf("요약 완료 checkpoint 조회 입력이 올바르지 않습니다")
	}
	checkpoint := publication.CompletionCheckpoint{}
	found := false
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var state model.PullRequestState
		err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("owner = ? AND repository = ? AND number = ?", target.Owner, target.Repository, target.Number).
			First(&state).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return publication.ErrLeaseLost
		}
		if err != nil {
			return err
		}
		leaseCurrentAt, err := databaseTime(transaction)
		if err != nil {
			return err
		}
		if !validSummaryPublicationLease(state, operationKey, leaseToken, leaseCurrentAt) {
			return publication.ErrLeaseLost
		}
		checkpoint = summaryCompletionCheckpoint(state)
		if state.SummaryResultInputHash != inputHash || state.SummaryResultCompletedAt == nil {
			return nil
		}
		if !validCompletionCheckpoint(checkpoint) {
			return fmt.Errorf("저장된 요약 완료 checkpoint가 유효하지 않습니다")
		}
		found = true
		return nil
	})
	if err != nil {
		return publication.CompletionCheckpoint{}, false, fmt.Errorf("요약 완료 checkpoint를 읽지 못했습니다: %w", err)
	}
	return checkpoint, found, nil
}

func (r *ReviewWorkflowRepository) RecordSummaryCompletionAttempt(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string, response llm.Response, recordedAt time.Time) (publication.CompletionCheckpoint, error) {
	if operationKey == "" || leaseToken == "" || recordedAt.IsZero() || !validCompletionAttempt(response) {
		return publication.CompletionCheckpoint{}, fmt.Errorf("요약 모델 시도 기록 입력이 올바르지 않습니다")
	}
	aggregate := publication.CompletionCheckpoint{}
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var state model.PullRequestState
		err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("owner = ? AND repository = ? AND number = ?", target.Owner, target.Repository, target.Number).
			First(&state).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return publication.ErrLeaseLost
		}
		if err != nil {
			return err
		}
		leaseCurrentAt, err := databaseTime(transaction)
		if err != nil {
			return err
		}
		if !validSummaryPublicationLease(state, operationKey, leaseToken, leaseCurrentAt) {
			return publication.ErrLeaseLost
		}
		aggregate = aggregateCompletionAttempt(summaryCompletionCheckpoint(state), response, false)
		updated := transaction.Model(&model.PullRequestState{}).Where("id = ?", state.ID).Updates(summaryCompletionMetadataValues(aggregate, recordedAt))
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return publication.ErrLeaseLost
		}
		return nil
	})
	if err != nil {
		return publication.CompletionCheckpoint{}, fmt.Errorf("요약 모델 시도 기록을 저장하지 못했습니다: %w", err)
	}
	return aggregate, nil
}

func (r *ReviewWorkflowRepository) SaveSummaryCompletion(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string, checkpoint publication.CompletionCheckpoint) (publication.CompletionCheckpoint, error) {
	if operationKey == "" || leaseToken == "" || !validCompletionCheckpoint(checkpoint) {
		return publication.CompletionCheckpoint{}, fmt.Errorf("요약 완료 checkpoint 입력이 올바르지 않습니다")
	}
	aggregate := publication.CompletionCheckpoint{}
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var state model.PullRequestState
		err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("owner = ? AND repository = ? AND number = ?", target.Owner, target.Repository, target.Number).
			First(&state).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return publication.ErrLeaseLost
		}
		if err != nil {
			return err
		}
		leaseCurrentAt, err := databaseTime(transaction)
		if err != nil {
			return err
		}
		if !validSummaryPublicationLease(state, operationKey, leaseToken, leaseCurrentAt) {
			return publication.ErrLeaseLost
		}
		response := llm.Response{
			Provider:       checkpoint.Provider,
			Model:          checkpoint.Model,
			ModelLabel:     checkpoint.ModelLabel,
			FinishReason:   checkpoint.FinishReason,
			Usage:          checkpoint.Usage,
			ToolExecutions: checkpoint.ToolExecutions,
		}
		aggregate = aggregateCompletionAttempt(summaryCompletionCheckpoint(state), response, checkpoint.MultipleModels)
		aggregate.InputHash = checkpoint.InputHash
		aggregate.CanonicalContent = checkpoint.CanonicalContent
		aggregate.FinishReason = checkpoint.FinishReason
		aggregate.CompletedAt = checkpoint.CompletedAt
		if !validCompletionCheckpoint(aggregate) {
			return fmt.Errorf("누적 요약 완료 checkpoint가 유효하지 않습니다")
		}
		values := summaryCompletionMetadataValues(aggregate, checkpoint.CompletedAt)
		values["summary_result_input_hash"] = aggregate.InputHash
		values["summary_canonical_content"] = aggregate.CanonicalContent
		values["summary_finish_reason"] = aggregate.FinishReason
		values["summary_result_completed_at"] = aggregate.CompletedAt
		updated := transaction.Model(&model.PullRequestState{}).Where("id = ?", state.ID).Updates(values)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return publication.ErrLeaseLost
		}
		return nil
	})
	if err != nil {
		return publication.CompletionCheckpoint{}, fmt.Errorf("요약 완료 checkpoint를 저장하지 못했습니다: %w", err)
	}
	return aggregate, nil
}

func (r *ReviewWorkflowRepository) RenewSummaryPublication(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string, leaseExpiresAt time.Time) error {
	updated := r.database.WithContext(ctx).Model(&model.PullRequestState{}).
		Where("owner = ? AND repository = ? AND number = ?", target.Owner, target.Repository, target.Number).
		Where("summary_operation_key = ? AND summary_publishing_key = ? AND summary_lease_token = ? AND summary_lease_expires_at > CURRENT_TIMESTAMP AND summary_expires_at > CURRENT_TIMESTAMP", operationKey, operationKey, leaseToken).
		UpdateColumn("summary_lease_expires_at", gorm.Expr("LEAST(?, summary_expires_at)", leaseExpiresAt))
	if updated.Error != nil {
		return fmt.Errorf("요약 게시 lease를 갱신하지 못했습니다: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return publication.ErrLeaseLost
	}
	return nil
}

func (r *ReviewWorkflowRepository) CompleteSummaryPublication(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string, completedAt time.Time) error {
	updated := r.database.WithContext(ctx).Model(&model.PullRequestState{}).
		Where("owner = ? AND repository = ? AND number = ?", target.Owner, target.Repository, target.Number).
		Where("summary_operation_key = ? AND summary_publishing_key = ? AND summary_lease_token = ? AND summary_lease_expires_at > CURRENT_TIMESTAMP AND summary_expires_at > CURRENT_TIMESTAMP", operationKey, operationKey, leaseToken).
		Updates(map[string]any{
			"summary_completed_key":    operationKey,
			"summary_completed_at":     completedAt,
			"summary_publishing_key":   "",
			"summary_lease_token":      "",
			"summary_lease_expires_at": nil,
			"updated_at":               completedAt,
		})
	if updated.Error != nil {
		return fmt.Errorf("요약 게시 완료를 저장하지 못했습니다: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return publication.ErrLeaseLost
	}
	return nil
}

func (r *ReviewWorkflowRepository) ReleaseSummaryPublication(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string) error {
	if operationKey == "" || leaseToken == "" {
		return nil
	}
	updated := r.database.WithContext(ctx).Model(&model.PullRequestState{}).
		Where("owner = ? AND repository = ? AND number = ?", target.Owner, target.Repository, target.Number).
		Where("summary_publishing_key = ? AND summary_lease_token = ?", operationKey, leaseToken).
		Updates(map[string]any{
			"summary_publishing_key":   "",
			"summary_lease_token":      "",
			"summary_lease_expires_at": nil,
		})
	if updated.Error != nil {
		return fmt.Errorf("요약 게시 lease를 해제하지 못했습니다: %w", updated.Error)
	}
	return nil
}

func summaryPublicationIsNewer(state model.PullRequestState, operationKey string, orderKey string, observedAt time.Time) bool {
	if state.SummaryObservedAt == nil {
		return false
	}
	if state.SummaryObservedAt.After(observedAt) {
		return true
	}
	if state.SummaryObservedAt.Before(observedAt) {
		return false
	}
	if state.SummaryOrderKey != orderKey {
		return state.SummaryOrderKey > orderKey
	}
	return state.SummaryOperationKey > operationKey
}

func summaryPublicationIsSame(state model.PullRequestState, operationKey string, orderKey string, observedAt time.Time) bool {
	return state.SummaryObservedAt != nil && state.SummaryObservedAt.Equal(observedAt) && state.SummaryOrderKey == orderKey && state.SummaryOperationKey == operationKey
}

func validSummaryPublicationLease(state model.PullRequestState, operationKey string, leaseToken string, currentAt time.Time) bool {
	return state.SummaryOperationKey == operationKey &&
		state.SummaryPublishingKey == operationKey &&
		state.SummaryLeaseToken == leaseToken &&
		state.SummaryLeaseExpiresAt != nil && state.SummaryLeaseExpiresAt.After(currentAt) &&
		state.SummaryExpiresAt != nil && state.SummaryExpiresAt.After(currentAt)
}

func summaryCompletionCheckpoint(state model.PullRequestState) publication.CompletionCheckpoint {
	completedAt := time.Time{}
	if state.SummaryResultCompletedAt != nil {
		completedAt = *state.SummaryResultCompletedAt
	}
	return publication.CompletionCheckpoint{
		InputHash:        state.SummaryResultInputHash,
		CanonicalContent: state.SummaryCanonicalContent,
		Provider:         state.SummaryProvider,
		Model:            state.SummaryModel,
		ModelLabel:       state.SummaryModelLabel,
		FinishReason:     state.SummaryFinishReason,
		MultipleModels:   state.SummaryMultipleModels,
		Usage: llm.Usage{
			PromptTokens:     state.SummaryPromptTokens,
			CompletionTokens: state.SummaryCompletionTokens,
			TotalTokens:      state.SummaryTotalTokens,
		},
		ToolExecutions: state.SummaryToolExecutions,
		CompletedAt:    completedAt,
	}
}

func summaryCompletionMetadataValues(checkpoint publication.CompletionCheckpoint, updatedAt time.Time) map[string]any {
	return map[string]any{
		"summary_provider":          checkpoint.Provider,
		"summary_model":             checkpoint.Model,
		"summary_model_label":       checkpoint.ModelLabel,
		"summary_multiple_models":   checkpoint.MultipleModels,
		"summary_prompt_tokens":     checkpoint.Usage.PromptTokens,
		"summary_completion_tokens": checkpoint.Usage.CompletionTokens,
		"summary_total_tokens":      checkpoint.Usage.TotalTokens,
		"summary_tool_executions":   checkpoint.ToolExecutions,
		"updated_at":                updatedAt,
	}
}

func summaryCompletionResetValues() map[string]any {
	return map[string]any{
		"summary_completed_key":       "",
		"summary_completed_at":        nil,
		"summary_result_input_hash":   "",
		"summary_canonical_content":   "",
		"summary_provider":            "",
		"summary_model":               "",
		"summary_model_label":         "",
		"summary_finish_reason":       "",
		"summary_multiple_models":     false,
		"summary_prompt_tokens":       0,
		"summary_completion_tokens":   0,
		"summary_total_tokens":        0,
		"summary_tool_executions":     0,
		"summary_result_completed_at": nil,
	}
}
