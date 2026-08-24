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

type ReplyPublicationRepository struct {
	database *gorm.DB
}

func NewReplyPublicationRepository(database *gorm.DB) *ReplyPublicationRepository {
	return &ReplyPublicationRepository{database: database}
}

func (r *ReplyPublicationRepository) ClaimReplyPublication(ctx context.Context, target pullrequest.Target, operationKey string, commentID int64, inThread bool, claimedAt time.Time, leaseExpiresAt time.Time, expiresAt time.Time) (publication.Claim, error) {
	if target.Owner == "" || target.Repository == "" || target.Number <= 0 || operationKey == "" || commentID <= 0 || claimedAt.IsZero() || !leaseExpiresAt.After(claimedAt) || !expiresAt.After(claimedAt) {
		return publication.Claim{}, fmt.Errorf("답글 게시 claim 입력이 올바르지 않습니다")
	}
	leaseToken, err := newLeaseToken()
	if err != nil {
		return publication.Claim{}, fmt.Errorf("답글 게시 lease를 만들지 못했습니다: %w", err)
	}
	claim := publication.Claim{LeaseToken: leaseToken}
	err = r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		entry := model.ReplyPublication{
			OperationKey: operationKey,
			Owner:        target.Owner,
			Repository:   target.Repository,
			Number:       target.Number,
			CommentID:    commentID,
			InThread:     inThread,
			ExpiresAt:    expiresAt,
			CreatedAt:    claimedAt,
			UpdatedAt:    claimedAt,
		}
		if err := transaction.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry).Error; err != nil {
			return err
		}
		entry = model.ReplyPublication{}
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_key = ?", operationKey).First(&entry).Error; err != nil {
			return err
		}
		if entry.Owner != target.Owner || entry.Repository != target.Repository || entry.Number != target.Number || entry.CommentID != commentID || entry.InThread != inThread {
			return fmt.Errorf("답글 게시 operation이 다른 대상에 연결되어 있습니다")
		}
		claimCurrentAt, err := databaseTime(transaction)
		if err != nil {
			return err
		}
		expired := !entry.ExpiresAt.After(claimCurrentAt)
		if !expired && entry.CompletedAt != nil {
			claim = publication.Claim{Completed: true}
			return nil
		}
		active := !expired && entry.LeaseToken != "" && entry.LeaseExpiresAt != nil && entry.LeaseExpiresAt.After(claimCurrentAt)
		if active {
			return publication.ErrLeased
		}
		lifecycleExpiresAt := expiresAt
		if !expired {
			lifecycleExpiresAt = entry.ExpiresAt
		}
		lifecycleLeaseExpiresAt := leaseExpiresAt
		if lifecycleLeaseExpiresAt.After(lifecycleExpiresAt) {
			lifecycleLeaseExpiresAt = lifecycleExpiresAt
		}
		if !lifecycleExpiresAt.After(claimCurrentAt) || !lifecycleLeaseExpiresAt.After(claimCurrentAt) {
			return fmt.Errorf("답글 게시 lifecycle이 이미 만료되었습니다")
		}
		externalCalls := entry.ExternalCalls
		if expired {
			externalCalls = 0
		}
		values := map[string]any{
			"lease_token":      leaseToken,
			"lease_expires_at": lifecycleLeaseExpiresAt,
			"completed_at":     nil,
			"expires_at":       lifecycleExpiresAt,
			"external_calls":   externalCalls,
			"updated_at":       claimedAt,
		}
		if expired {
			mergeValues(values, replyCompletionResetValues())
		}
		updated := transaction.Model(&model.ReplyPublication{}).Where("id = ?", entry.ID).Updates(values)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return publication.ErrLeaseLost
		}
		return nil
	})
	if err != nil {
		return publication.Claim{}, fmt.Errorf("답글 게시 권한을 얻지 못했습니다: %w", err)
	}
	return claim, nil
}

func (r *ReplyPublicationRepository) ReserveReplyExternalCall(ctx context.Context, operationKey string, leaseToken string, limit int) (bool, error) {
	if limit < 1 {
		return false, nil
	}
	updated := r.database.WithContext(ctx).Model(&model.ReplyPublication{}).
		Where("operation_key = ? AND lease_token = ? AND completed_at IS NULL AND lease_expires_at > CURRENT_TIMESTAMP AND expires_at > CURRENT_TIMESTAMP AND COALESCE(external_calls, 0) < ?", operationKey, leaseToken, limit).
		UpdateColumn("external_calls", gorm.Expr("COALESCE(external_calls, 0) + 1"))
	if updated.Error != nil {
		return false, fmt.Errorf("답글 외부 호출 예산을 예약하지 못했습니다: %w", updated.Error)
	}
	return updated.RowsAffected == 1, nil
}

func (r *ReplyPublicationRepository) ReplyCompletion(ctx context.Context, operationKey string, leaseToken string, inputHash string, currentAt time.Time) (publication.CompletionCheckpoint, bool, error) {
	if operationKey == "" || leaseToken == "" || inputHash == "" || currentAt.IsZero() {
		return publication.CompletionCheckpoint{}, false, fmt.Errorf("답글 완료 checkpoint 조회 입력이 올바르지 않습니다")
	}
	checkpoint := publication.CompletionCheckpoint{}
	found := false
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var entry model.ReplyPublication
		err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_key = ?", operationKey).First(&entry).Error
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
		if !validReplyPublicationLease(entry, leaseToken, leaseCurrentAt) {
			return publication.ErrLeaseLost
		}
		checkpoint = replyCompletionCheckpoint(entry)
		if entry.ResultInputHash != inputHash || entry.ResultCompletedAt == nil {
			return nil
		}
		if !validCompletionCheckpoint(checkpoint) {
			return fmt.Errorf("저장된 답글 완료 checkpoint가 유효하지 않습니다")
		}
		found = true
		return nil
	})
	if err != nil {
		return publication.CompletionCheckpoint{}, false, fmt.Errorf("답글 완료 checkpoint를 읽지 못했습니다: %w", err)
	}
	return checkpoint, found, nil
}

func (r *ReplyPublicationRepository) RecordReplyCompletionAttempt(ctx context.Context, operationKey string, leaseToken string, response llm.Response, recordedAt time.Time) (publication.CompletionCheckpoint, error) {
	if operationKey == "" || leaseToken == "" || recordedAt.IsZero() || !validCompletionAttempt(response) {
		return publication.CompletionCheckpoint{}, fmt.Errorf("답글 모델 시도 기록 입력이 올바르지 않습니다")
	}
	aggregate := publication.CompletionCheckpoint{}
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var entry model.ReplyPublication
		err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_key = ?", operationKey).First(&entry).Error
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
		if !validReplyPublicationLease(entry, leaseToken, leaseCurrentAt) {
			return publication.ErrLeaseLost
		}
		aggregate = aggregateCompletionAttempt(replyCompletionCheckpoint(entry), response, false)
		updated := transaction.Model(&model.ReplyPublication{}).Where("id = ?", entry.ID).Updates(replyCompletionMetadataValues(aggregate, recordedAt))
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return publication.ErrLeaseLost
		}
		return nil
	})
	if err != nil {
		return publication.CompletionCheckpoint{}, fmt.Errorf("답글 모델 시도 기록을 저장하지 못했습니다: %w", err)
	}
	return aggregate, nil
}

func (r *ReplyPublicationRepository) SaveReplyCompletion(ctx context.Context, operationKey string, leaseToken string, checkpoint publication.CompletionCheckpoint) (publication.CompletionCheckpoint, error) {
	if operationKey == "" || leaseToken == "" || !validCompletionCheckpoint(checkpoint) {
		return publication.CompletionCheckpoint{}, fmt.Errorf("답글 완료 checkpoint 입력이 올바르지 않습니다")
	}
	aggregate := publication.CompletionCheckpoint{}
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var entry model.ReplyPublication
		err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_key = ?", operationKey).First(&entry).Error
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
		if !validReplyPublicationLease(entry, leaseToken, leaseCurrentAt) {
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
		aggregate = aggregateCompletionAttempt(replyCompletionCheckpoint(entry), response, checkpoint.MultipleModels)
		aggregate.InputHash = checkpoint.InputHash
		aggregate.CanonicalContent = checkpoint.CanonicalContent
		aggregate.FinishReason = checkpoint.FinishReason
		aggregate.CompletedAt = checkpoint.CompletedAt
		if !validCompletionCheckpoint(aggregate) {
			return fmt.Errorf("누적 답글 완료 checkpoint가 유효하지 않습니다")
		}
		values := replyCompletionMetadataValues(aggregate, checkpoint.CompletedAt)
		values["result_input_hash"] = aggregate.InputHash
		values["canonical_content"] = aggregate.CanonicalContent
		values["finish_reason"] = aggregate.FinishReason
		values["result_completed_at"] = aggregate.CompletedAt
		updated := transaction.Model(&model.ReplyPublication{}).Where("id = ?", entry.ID).Updates(values)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return publication.ErrLeaseLost
		}
		return nil
	})
	if err != nil {
		return publication.CompletionCheckpoint{}, fmt.Errorf("답글 완료 checkpoint를 저장하지 못했습니다: %w", err)
	}
	return aggregate, nil
}

func (r *ReplyPublicationRepository) RenewReplyPublication(ctx context.Context, operationKey string, leaseToken string, leaseExpiresAt time.Time) error {
	updated := r.database.WithContext(ctx).Model(&model.ReplyPublication{}).
		Where("operation_key = ? AND lease_token = ? AND completed_at IS NULL AND lease_expires_at > CURRENT_TIMESTAMP AND expires_at > CURRENT_TIMESTAMP", operationKey, leaseToken).
		UpdateColumn("lease_expires_at", gorm.Expr("LEAST(?, expires_at)", leaseExpiresAt))
	if updated.Error != nil {
		return fmt.Errorf("답글 게시 lease를 갱신하지 못했습니다: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return publication.ErrLeaseLost
	}
	return nil
}

func (r *ReplyPublicationRepository) CompleteReplyPublication(ctx context.Context, operationKey string, leaseToken string, completedAt time.Time) error {
	updated := r.database.WithContext(ctx).Model(&model.ReplyPublication{}).
		Where("operation_key = ? AND lease_token = ? AND completed_at IS NULL AND lease_expires_at > CURRENT_TIMESTAMP AND expires_at > CURRENT_TIMESTAMP", operationKey, leaseToken).
		Updates(map[string]any{
			"lease_token":      "",
			"lease_expires_at": nil,
			"completed_at":     completedAt,
			"updated_at":       completedAt,
		})
	if updated.Error != nil {
		return fmt.Errorf("답글 게시 완료를 저장하지 못했습니다: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return publication.ErrLeaseLost
	}
	return nil
}

func (r *ReplyPublicationRepository) ReleaseReplyPublication(ctx context.Context, operationKey string, leaseToken string) error {
	if operationKey == "" || leaseToken == "" {
		return nil
	}
	updated := r.database.WithContext(ctx).Model(&model.ReplyPublication{}).
		Where("operation_key = ? AND lease_token = ? AND completed_at IS NULL", operationKey, leaseToken).
		UpdateColumns(map[string]any{
			"lease_token":      "",
			"lease_expires_at": nil,
		})
	if updated.Error != nil {
		return fmt.Errorf("답글 게시 lease를 해제하지 못했습니다: %w", updated.Error)
	}
	return nil
}

func validReplyPublicationLease(entry model.ReplyPublication, leaseToken string, currentAt time.Time) bool {
	return entry.LeaseToken == leaseToken &&
		entry.CompletedAt == nil &&
		entry.LeaseExpiresAt != nil && entry.LeaseExpiresAt.After(currentAt) &&
		entry.ExpiresAt.After(currentAt)
}

func replyCompletionCheckpoint(entry model.ReplyPublication) publication.CompletionCheckpoint {
	completedAt := time.Time{}
	if entry.ResultCompletedAt != nil {
		completedAt = *entry.ResultCompletedAt
	}
	return publication.CompletionCheckpoint{
		InputHash:        entry.ResultInputHash,
		CanonicalContent: entry.CanonicalContent,
		Provider:         entry.Provider,
		Model:            entry.Model,
		ModelLabel:       entry.ModelLabel,
		FinishReason:     entry.FinishReason,
		MultipleModels:   entry.MultipleModels,
		Usage: llm.Usage{
			PromptTokens:     entry.PromptTokens,
			CompletionTokens: entry.CompletionTokens,
			TotalTokens:      entry.TotalTokens,
		},
		ToolExecutions: entry.ToolExecutions,
		CompletedAt:    completedAt,
	}
}

func replyCompletionMetadataValues(checkpoint publication.CompletionCheckpoint, updatedAt time.Time) map[string]any {
	return map[string]any{
		"provider":          checkpoint.Provider,
		"model":             checkpoint.Model,
		"model_label":       checkpoint.ModelLabel,
		"multiple_models":   checkpoint.MultipleModels,
		"prompt_tokens":     checkpoint.Usage.PromptTokens,
		"completion_tokens": checkpoint.Usage.CompletionTokens,
		"total_tokens":      checkpoint.Usage.TotalTokens,
		"tool_executions":   checkpoint.ToolExecutions,
		"updated_at":        updatedAt,
	}
}

func replyCompletionResetValues() map[string]any {
	return map[string]any{
		"completed_at":        nil,
		"result_input_hash":   "",
		"canonical_content":   "",
		"provider":            "",
		"model":               "",
		"model_label":         "",
		"finish_reason":       "",
		"multiple_models":     false,
		"prompt_tokens":       0,
		"completion_tokens":   0,
		"total_tokens":        0,
		"tool_executions":     0,
		"result_completed_at": nil,
	}
}
