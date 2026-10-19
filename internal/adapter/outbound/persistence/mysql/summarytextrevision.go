package mysql

import (
	"context"
	"errors"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/textrevision"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *SummaryPublicationRepository) SummaryTextRevision(ctx context.Context, target pullrequest.Target, operationKey string, lease string, inputHash string) (textrevision.Checkpoint, bool, error) {
	checkpoint := textrevision.Checkpoint{}
	found := false
	err := r.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, err := lockedSummaryRevisionState(tx, target, operationKey, lease)
		if err != nil {
			return err
		}
		if state.SummaryTextRevisionJSON == "" {
			return nil
		}
		checkpoint, err = textrevision.DecodeCheckpoint(state.SummaryTextRevisionJSON)
		found = err == nil && checkpoint.InputHash == inputHash
		return err
	})
	return checkpoint, found, err
}

func (r *SummaryPublicationRepository) SaveSummaryTextRevision(ctx context.Context, target pullrequest.Target, operationKey string, lease string, checkpoint textrevision.Checkpoint) error {
	encoded, err := checkpoint.Encode()
	if err != nil {
		return err
	}
	return r.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, err := lockedSummaryRevisionState(tx, target, operationKey, lease)
		if err != nil {
			return err
		}
		if state.SummaryTextRevisionJSON != "" {
			prior, err := textrevision.DecodeCheckpoint(state.SummaryTextRevisionJSON)
			if err != nil {
				return err
			}
			if prior.InputHash == checkpoint.InputHash {
				if state.SummaryTextRevisionJSON != encoded {
					return errors.New("이미 확정된 요약 문체 보정 결과를 변경할 수 없습니다")
				}
				return nil
			}
		}
		return tx.Model(&state).Update("summary_text_revision_json", encoded).Error
	})
}

func lockedSummaryRevisionState(tx *gorm.DB, target pullrequest.Target, operationKey string, lease string) (model.PullRequestState, error) {
	var state model.PullRequestState
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("owner = ? AND repository = ? AND number = ?", target.Owner, target.Repository, target.Number).First(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return state, publication.ErrLeaseLost
	}
	if err != nil {
		return state, err
	}
	currentAt, err := databaseTime(tx)
	if err != nil {
		return state, err
	}
	if !validSummaryPublicationLease(state, operationKey, lease, currentAt) {
		return state, summaryPublicationLeaseStateError(state, operationKey)
	}
	if state.SummaryResultCompletedAt == nil {
		return state, errors.New("보정할 요약 완료 결과가 없습니다")
	}
	return state, nil
}
