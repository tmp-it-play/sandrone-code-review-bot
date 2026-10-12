package mysql

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewsummary"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ReviewSummaryStore struct{ database *gorm.DB }

func NewReviewSummaryStore(database *gorm.DB) *ReviewSummaryStore {
	return &ReviewSummaryStore{database: database}
}

func (r *ReviewSummaryStore) ReviewSummary(ctx context.Context, runID uint64, lease string) (reviewworkflow.SummaryCheckpoint, bool, error) {
	checkpoint := reviewworkflow.SummaryCheckpoint{}
	found := false
	err := r.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireRunLease(tx, runID, lease); err != nil {
			return err
		}
		var entry model.ReviewSummary
		err := tx.Where("review_run_id = ?", runID).First(&entry).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(entry.CheckpointJSON), &checkpoint); err != nil {
			return err
		}
		if entry.InputHash != checkpoint.InputHash || !validSummaryCheckpoint(checkpoint) {
			return errors.New("저장된 최종 요약이 유효하지 않습니다")
		}
		found = true
		return nil
	})
	return checkpoint, found, err
}

func (r *ReviewSummaryStore) SaveReviewSummary(ctx context.Context, runID uint64, lease string, checkpoint reviewworkflow.SummaryCheckpoint) error {
	if !validSummaryCheckpoint(checkpoint) {
		return errors.New("최종 요약 checkpoint가 유효하지 않습니다")
	}
	encoded, err := json.Marshal(checkpoint)
	if err != nil {
		return err
	}
	return r.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireRunLease(tx, runID, lease); err != nil {
			return err
		}
		var prior model.ReviewSummary
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("review_run_id = ?", runID).First(&prior).Error
		if err == nil {
			if prior.InputHash != checkpoint.InputHash || prior.CheckpointJSON != string(encoded) {
				return reviewworkflow.ErrPlanChanged
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return tx.Create(&model.ReviewSummary{ReviewRunID: runID, InputHash: checkpoint.InputHash, CheckpointJSON: string(encoded)}).Error
	})
}

func validSummaryCheckpoint(value reviewworkflow.SummaryCheckpoint) bool {
	_, err := hex.DecodeString(value.InputHash)
	return err == nil && len(value.InputHash) == 64 && strings.TrimSpace(value.Overview) != "" && utf8.RuneCountInString(value.Overview) <= reviewsummary.MaxCharacters && value.Response.Usage.PromptTokens >= 0 && value.Response.Usage.CompletionTokens >= 0 && value.Response.Usage.TotalTokens >= 0
}
