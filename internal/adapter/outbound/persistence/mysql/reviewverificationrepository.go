package mysql

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *ReviewVerificationStore) ReviewVerification(ctx context.Context, runID uint64, runLeaseToken string, inputHash string) (reviewworkflow.VerificationCheckpoint, bool, error) {
	checkpoint := reviewworkflow.VerificationCheckpoint{}
	found := false
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := requireRunLease(transaction, runID, runLeaseToken); err != nil {
			return err
		}
		var entry model.ReviewVerification
		err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("review_run_id = ?", runID).First(&entry).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var supported []string
		if err := json.Unmarshal([]byte(entry.SupportedOccurrenceIDsJSON), &supported); err != nil {
			return fmt.Errorf("finding verifier checkpoint occurrence ID를 읽지 못했습니다: %w", err)
		}
		checkpoint = verificationCheckpointFromEntry(entry, supported)
		if !validStoredVerification(checkpoint) {
			return errors.New("저장된 finding verifier checkpoint가 유효하지 않습니다")
		}
		found = checkpoint.Completed && checkpoint.InputHash == inputHash
		return nil
	})
	if err != nil {
		return reviewworkflow.VerificationCheckpoint{}, false, fmt.Errorf("finding verifier checkpoint를 읽지 못했습니다: %w", err)
	}
	return checkpoint, found, nil
}

func (r *ReviewVerificationStore) RecordReviewVerificationAttempt(ctx context.Context, runID uint64, runLeaseToken string, inputHash string, response llm.Response, finishedAt time.Time) (reviewworkflow.VerificationCheckpoint, error) {
	if runID == 0 || !validVerificationHash(inputHash) || finishedAt.IsZero() || !validVerificationResponse(response) {
		return reviewworkflow.VerificationCheckpoint{}, errors.New("finding verifier 시도 기록이 완전하지 않습니다")
	}
	aggregate := reviewworkflow.VerificationCheckpoint{}
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := requireRunLease(transaction, runID, runLeaseToken); err != nil {
			return err
		}
		entry, prior, err := lockedReviewVerification(transaction, runID)
		if err != nil {
			return err
		}
		aggregate = aggregateVerificationCheckpoint(prior, response, finishedAt)
		aggregate.AttemptCount++
		aggregate.LastAttemptInputHash = inputHash
		aggregate.LastAttemptSucceeded = false
		return transaction.Model(&model.ReviewVerification{}).Where("id = ?", entry.ID).Updates(verificationMetadataValues(aggregate)).Error
	})
	if err != nil {
		return reviewworkflow.VerificationCheckpoint{}, fmt.Errorf("finding verifier 시도 기록을 저장하지 못했습니다: %w", err)
	}
	return aggregate, nil
}

func (r *ReviewVerificationStore) SaveReviewVerification(ctx context.Context, runID uint64, runLeaseToken string, checkpoint reviewworkflow.VerificationCheckpoint) (reviewworkflow.VerificationCheckpoint, error) {
	if checkpoint.RunID != runID || checkpoint.InputHash == "" || checkpoint.FinishedAt.IsZero() {
		return reviewworkflow.VerificationCheckpoint{}, errors.New("finding verifier checkpoint가 완전하지 않습니다")
	}
	supported := append([]string{}, checkpoint.SupportedOccurrenceIDs...)
	slices.Sort(supported)
	if !validStoredOccurrenceIDs(supported) {
		return reviewworkflow.VerificationCheckpoint{}, errors.New("finding verifier checkpoint occurrence ID가 유효하지 않습니다")
	}
	encoded, err := json.Marshal(supported)
	if err != nil {
		return reviewworkflow.VerificationCheckpoint{}, fmt.Errorf("finding verifier checkpoint를 직렬화하지 못했습니다: %w", err)
	}
	aggregate := reviewworkflow.VerificationCheckpoint{}
	err = r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := requireRunLease(transaction, runID, runLeaseToken); err != nil {
			return err
		}
		entry, prior, err := lockedReviewVerification(transaction, runID)
		if err != nil {
			return err
		}
		aggregate = aggregateVerificationCheckpoint(prior, checkpoint.MetadataResponse(), checkpoint.FinishedAt)
		aggregate.AttemptCount++
		aggregate.InputHash = checkpoint.InputHash
		aggregate.SupportedOccurrenceIDs = supported
		aggregate.Completed = true
		aggregate.LastAttemptInputHash = checkpoint.InputHash
		aggregate.LastAttemptSucceeded = true
		aggregate.CompletedAt = checkpoint.FinishedAt
		if !validStoredVerification(aggregate) {
			return errors.New("finding verifier checkpoint metadata가 유효하지 않습니다")
		}
		values := verificationMetadataValues(aggregate)
		values["input_hash"] = aggregate.InputHash
		values["supported_occurrence_ids_json"] = string(encoded)
		values["completed"] = true
		values["result_completed_at"] = aggregate.CompletedAt
		return transaction.Model(&model.ReviewVerification{}).Where("id = ?", entry.ID).Updates(values).Error
	})
	if err != nil {
		return reviewworkflow.VerificationCheckpoint{}, fmt.Errorf("finding verifier checkpoint를 저장하지 못했습니다: %w", err)
	}
	return aggregate, nil
}

func lockedReviewVerification(transaction *gorm.DB, runID uint64) (model.ReviewVerification, reviewworkflow.VerificationCheckpoint, error) {
	var entry model.ReviewVerification
	err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("review_run_id = ?", runID).First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		entry = model.ReviewVerification{
			ReviewRunID:                runID,
			InputHash:                  "",
			SupportedOccurrenceIDsJSON: "[]",
		}
		if err := transaction.Create(&entry).Error; err != nil {
			return model.ReviewVerification{}, reviewworkflow.VerificationCheckpoint{}, err
		}
		return entry, reviewworkflow.VerificationCheckpoint{RunID: runID, SupportedOccurrenceIDs: []string{}}, nil
	}
	if err != nil {
		return model.ReviewVerification{}, reviewworkflow.VerificationCheckpoint{}, err
	}
	var supported []string
	if err := json.Unmarshal([]byte(entry.SupportedOccurrenceIDsJSON), &supported); err != nil {
		return model.ReviewVerification{}, reviewworkflow.VerificationCheckpoint{}, err
	}
	prior := verificationCheckpointFromEntry(entry, supported)
	if !validStoredVerification(prior) {
		return model.ReviewVerification{}, reviewworkflow.VerificationCheckpoint{}, errors.New("저장된 finding verifier checkpoint가 유효하지 않습니다")
	}
	return entry, prior, nil
}

func aggregateVerificationCheckpoint(prior reviewworkflow.VerificationCheckpoint, response llm.Response, finishedAt time.Time) reviewworkflow.VerificationCheckpoint {
	priorIdentity := prior.Provider != "" || prior.Model != ""
	responseIdentity := response.Provider != "" || response.Model != ""
	multiple := prior.MultipleModels || response.ModelLabel == "복수 모델" || response.Provider == "multiple" || response.Model == "multiple"
	if priorIdentity && responseIdentity && (prior.Provider != response.Provider || prior.Model != response.Model) {
		multiple = true
	}
	if response.Provider != "" {
		prior.Provider = response.Provider
	}
	if response.Model != "" {
		prior.Model = response.Model
	}
	prior.MultipleModels = multiple
	prior.Usage = normalizeVerificationUsage(prior.Usage).Add(normalizeVerificationUsage(response.Usage))
	prior.ToolExecutions += response.ToolExecutions
	prior.FinishedAt = finishedAt
	return prior
}

func verificationCheckpointFromEntry(entry model.ReviewVerification, supported []string) reviewworkflow.VerificationCheckpoint {
	completedAt := time.Time{}
	if entry.ResultCompletedAt != nil {
		completedAt = *entry.ResultCompletedAt
	}
	return reviewworkflow.VerificationCheckpoint{
		RunID:                  entry.ReviewRunID,
		InputHash:              entry.InputHash,
		SupportedOccurrenceIDs: append([]string{}, supported...),
		Provider:               entry.Provider,
		Model:                  entry.Model,
		MultipleModels:         entry.MultipleModels,
		Usage: llm.Usage{
			PromptTokens:     entry.PromptTokens,
			CompletionTokens: entry.CompletionTokens,
			TotalTokens:      entry.TotalTokens,
		},
		ToolExecutions:       entry.ToolExecutions,
		AttemptCount:         entry.AttemptCount,
		Completed:            entry.Completed,
		LastAttemptInputHash: entry.LastAttemptInputHash,
		LastAttemptSucceeded: entry.LastAttemptSucceeded,
		CompletedAt:          completedAt,
		FinishedAt:           entry.FinishedAt,
	}
}

func verificationMetadataValues(checkpoint reviewworkflow.VerificationCheckpoint) map[string]any {
	return map[string]any{
		"provider":                checkpoint.Provider,
		"model":                   checkpoint.Model,
		"multiple_models":         checkpoint.MultipleModels,
		"prompt_tokens":           checkpoint.Usage.PromptTokens,
		"completion_tokens":       checkpoint.Usage.CompletionTokens,
		"total_tokens":            checkpoint.Usage.TotalTokens,
		"tool_executions":         checkpoint.ToolExecutions,
		"attempt_count":           checkpoint.AttemptCount,
		"last_attempt_input_hash": checkpoint.LastAttemptInputHash,
		"last_attempt_succeeded":  checkpoint.LastAttemptSucceeded,
		"finished_at":             checkpoint.FinishedAt,
	}
}

func normalizeVerificationUsage(value llm.Usage) llm.Usage {
	if value.TotalTokens == 0 && (value.PromptTokens > 0 || value.CompletionTokens > 0) {
		value.TotalTokens = value.PromptTokens + value.CompletionTokens
	}
	return value
}

func validVerificationResponse(response llm.Response) bool {
	return response.Usage.PromptTokens >= 0 && response.Usage.CompletionTokens >= 0 && response.Usage.TotalTokens >= 0 && response.ToolExecutions >= 0
}

func validVerificationHash(inputHash string) bool {
	if len(inputHash) != 64 {
		return false
	}
	_, err := hex.DecodeString(inputHash)
	return err == nil
}

func validStoredOccurrenceIDs(identifiers []string) bool {
	if identifiers == nil {
		return false
	}
	seen := make(map[string]struct{}, len(identifiers))
	for _, identifier := range identifiers {
		if len(identifier) != 64 {
			return false
		}
		if _, err := hex.DecodeString(identifier); err != nil {
			return false
		}
		if _, exists := seen[identifier]; exists {
			return false
		}
		seen[identifier] = struct{}{}
	}
	return true
}

func validStoredVerification(checkpoint reviewworkflow.VerificationCheckpoint) bool {
	if checkpoint.RunID == 0 || checkpoint.AttemptCount < 0 || checkpoint.Usage.PromptTokens < 0 || checkpoint.Usage.CompletionTokens < 0 || checkpoint.Usage.TotalTokens < 0 || checkpoint.ToolExecutions < 0 {
		return false
	}
	if checkpoint.AttemptCount > 0 && checkpoint.FinishedAt.IsZero() {
		return false
	}
	if checkpoint.AttemptCount > 0 && !validVerificationHash(checkpoint.LastAttemptInputHash) {
		return false
	}
	if !checkpoint.Completed {
		return true
	}
	if !validVerificationHash(checkpoint.InputHash) || checkpoint.Provider == "" || checkpoint.Model == "" || checkpoint.FinishedAt.IsZero() || checkpoint.CompletedAt.IsZero() {
		return false
	}
	return validStoredOccurrenceIDs(checkpoint.SupportedOccurrenceIDs)
}
