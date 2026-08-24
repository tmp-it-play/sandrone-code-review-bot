package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *ReviewWorkflowRepository) ReviewPublication(ctx context.Context, runID uint64, runLeaseToken string) (reviewworkflow.ReviewPublication, bool, error) {
	publication := reviewworkflow.ReviewPublication{}
	found := false
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := requireRunLease(transaction, runID, runLeaseToken); err != nil {
			return err
		}
		now, err := databaseTime(transaction)
		if err != nil {
			return err
		}
		var entry model.ReviewPublication
		if err := transaction.Where("review_run_id = ?", runID).First(&entry).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if !entry.ExpiresAt.After(now) {
			return reviewworkflow.ErrPublicationExpired
		}
		decoded, err := decodeReviewPublication(entry)
		if err != nil {
			return err
		}
		publication = decoded
		found = true
		return nil
	})
	if err != nil {
		return reviewworkflow.ReviewPublication{}, false, fmt.Errorf("리뷰 게시 payload를 읽지 못했습니다: %w", err)
	}
	return publication, found, nil
}

func (r *ReviewWorkflowRepository) PrepareReviewPublication(ctx context.Context, runID uint64, runLeaseToken string, marker string, payload reviewworkflow.ReviewPublicationPayload, finalization reviewworkflow.ReviewPublicationFinalization, preparedAt time.Time, expiresAt time.Time) (reviewworkflow.ReviewPublication, error) {
	payload = payload.Bounded(marker)
	if err := payload.Validate(marker); err != nil {
		return reviewworkflow.ReviewPublication{}, err
	}
	if err := finalization.Validate(); err != nil {
		return reviewworkflow.ReviewPublication{}, err
	}
	if preparedAt.IsZero() || !expiresAt.After(preparedAt) {
		return reviewworkflow.ReviewPublication{}, errors.New("리뷰 게시 payload 보존 기간이 올바르지 않습니다")
	}
	payloadHash, err := payload.Hash()
	if err != nil {
		return reviewworkflow.ReviewPublication{}, fmt.Errorf("리뷰 게시 payload hash를 만들지 못했습니다: %w", err)
	}
	commentsJSON, err := json.Marshal(payload.Comments)
	if err != nil {
		return reviewworkflow.ReviewPublication{}, fmt.Errorf("인라인 리뷰 코멘트를 직렬화하지 못했습니다: %w", err)
	}
	publication := reviewworkflow.ReviewPublication{}
	err = r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := requireReviewPublicationLease(transaction, runID, runLeaseToken); err != nil {
			return err
		}
		now, err := databaseTime(transaction)
		if err != nil {
			return err
		}
		if !expiresAt.After(now) {
			return reviewworkflow.ErrPublicationExpired
		}
		var existing model.ReviewPublication
		existingErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("review_run_id = ?", runID).First(&existing).Error
		if existingErr == nil {
			if !existing.ExpiresAt.After(now) {
				return reviewworkflow.ErrPublicationExpired
			}
			if existing.Marker != marker {
				return errors.New("같은 리뷰 실행의 게시 marker가 변경되었습니다")
			}
			if existing.Status == reviewworkflow.ReviewPublicationStatusPrepared && existing.Body == "" && existing.FallbackBody == "" {
				if existing.PayloadHash != payloadHash {
					return errors.New("기존 리뷰 게시 hash에 대응하는 canonical payload가 없습니다")
				}
				if err := transaction.Model(&model.ReviewPublication{}).Where("id = ? AND status = ?", existing.ID, reviewworkflow.ReviewPublicationStatusPrepared).Updates(map[string]any{
					"body":          payload.Body,
					"comments_json": string(commentsJSON),
					"fallback_body": payload.FallbackBody,
					"updated_at":    preparedAt,
				}).Error; err != nil {
					return err
				}
				existing.Body = payload.Body
				existing.CommentsJSON = string(commentsJSON)
				existing.FallbackBody = payload.FallbackBody
				existing.UpdatedAt = preparedAt
			}
			if existing.Status == reviewworkflow.ReviewPublicationStatusPrepared && existing.IntendedStatus == "" {
				if err := transaction.Model(&model.ReviewPublication{}).Where("id = ? AND status = ?", existing.ID, reviewworkflow.ReviewPublicationStatusPrepared).Updates(map[string]any{
					"intended_status":   string(finalization.Status),
					"final_detail":      finalization.Detail,
					"advance_watermark": finalization.AdvanceWatermark,
					"updated_at":        preparedAt,
				}).Error; err != nil {
					return err
				}
				existing.IntendedStatus = string(finalization.Status)
				existing.FinalDetail = finalization.Detail
				existing.AdvanceWatermark = finalization.AdvanceWatermark
				existing.UpdatedAt = preparedAt
			}
			decoded, err := decodeReviewPublication(existing)
			if err != nil {
				return err
			}
			publication = decoded
			return nil
		}
		if !errors.Is(existingErr, gorm.ErrRecordNotFound) {
			return existingErr
		}
		entry := model.ReviewPublication{
			ReviewRunID:      runID,
			PayloadHash:      payloadHash,
			Marker:           marker,
			Body:             payload.Body,
			CommentsJSON:     string(commentsJSON),
			FallbackBody:     payload.FallbackBody,
			IntendedStatus:   string(finalization.Status),
			FinalDetail:      finalization.Detail,
			AdvanceWatermark: finalization.AdvanceWatermark,
			Status:           reviewworkflow.ReviewPublicationStatusPrepared,
			PreparedAt:       preparedAt,
			ExpiresAt:        expiresAt,
			CreatedAt:        preparedAt,
			UpdatedAt:        preparedAt,
		}
		if err := transaction.Create(&entry).Error; err != nil {
			return err
		}
		publication = reviewworkflow.ReviewPublication{
			RunID:        runID,
			PayloadHash:  payloadHash,
			Marker:       marker,
			Payload:      payload,
			Finalization: finalization,
			Status:       reviewworkflow.ReviewPublicationStatusPrepared,
			PreparedAt:   preparedAt,
			ExpiresAt:    expiresAt,
		}
		return nil
	})
	if err != nil {
		return reviewworkflow.ReviewPublication{}, fmt.Errorf("리뷰 게시 payload를 준비하지 못했습니다: %w", err)
	}
	return publication, nil
}

func (r *ReviewWorkflowRepository) CompleteReviewPublication(ctx context.Context, runID uint64, runLeaseToken string, marker string, channel string, externalID int64, completedAt time.Time, expiresAt time.Time) error {
	if strings.TrimSpace(marker) == "" || completedAt.IsZero() || !expiresAt.After(completedAt) || !reviewworkflow.ValidReviewPublicationChannel(channel, externalID) {
		return errors.New("리뷰 게시 receipt가 올바르지 않습니다")
	}
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := requireReviewPublicationLease(transaction, runID, runLeaseToken); err != nil {
			return err
		}
		now, err := databaseTime(transaction)
		if err != nil {
			return err
		}
		if !expiresAt.After(now) {
			return reviewworkflow.ErrPublicationExpired
		}
		var publication model.ReviewPublication
		entryErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("review_run_id = ?", runID).First(&publication).Error
		if errors.Is(entryErr, gorm.ErrRecordNotFound) {
			if channel != reviewworkflow.ReviewPublicationChannelReconciled {
				return reviewworkflow.ErrPublicationIncomplete
			}
			legacy := model.ReviewPublication{
				ReviewRunID:  runID,
				PayloadHash:  "",
				Marker:       marker,
				Body:         "",
				CommentsJSON: "[]",
				FallbackBody: "",
				Status:       reviewworkflow.ReviewPublicationStatusCompleted,
				Channel:      channel,
				ExternalID:   externalID,
				PreparedAt:   completedAt,
				CompletedAt:  &completedAt,
				ExpiresAt:    expiresAt,
				CreatedAt:    completedAt,
				UpdatedAt:    completedAt,
			}
			return transaction.Create(&legacy).Error
		}
		if entryErr != nil {
			return entryErr
		}
		if !publication.ExpiresAt.After(now) {
			return reviewworkflow.ErrPublicationExpired
		}
		if publication.Marker != marker {
			return errors.New("리뷰 게시 receipt의 marker가 일치하지 않습니다")
		}
		if completedAt.Before(publication.PreparedAt) || !publication.ExpiresAt.After(completedAt) {
			return reviewworkflow.ErrPublicationExpired
		}
		decoded, err := decodeReviewPublication(publication)
		if err != nil {
			return err
		}
		if decoded.Status == reviewworkflow.ReviewPublicationStatusCompleted {
			return nil
		}
		updated := transaction.Model(&model.ReviewPublication{}).
			Where("review_run_id = ? AND status = ?", runID, reviewworkflow.ReviewPublicationStatusPrepared).
			Updates(map[string]any{
				"status":       reviewworkflow.ReviewPublicationStatusCompleted,
				"channel":      channel,
				"external_id":  externalID,
				"completed_at": completedAt,
				"updated_at":   completedAt,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return reviewworkflow.ErrPublicationIncomplete
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("리뷰 게시 결과를 완료하지 못했습니다: %w", err)
	}
	return nil
}

func requireReviewPublicationLease(transaction *gorm.DB, runID uint64, runLeaseToken string) error {
	var run model.ReviewRun
	if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, runID).Error; err != nil {
		return err
	}
	if err := validateRunLease(transaction, run, runLeaseToken); err != nil {
		return err
	}
	if reviewworkflow.RunStatus(run.Status) != reviewworkflow.RunStatusPublishing {
		return reviewworkflow.ErrPublicationLeased
	}
	var state model.PullRequestState
	if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("owner = ? AND repository = ? AND number = ?", run.Owner, run.Repository, run.Number).First(&state).Error; err != nil {
		return err
	}
	now, err := databaseTime(transaction)
	if err != nil {
		return err
	}
	owned := state.LatestRunID == run.ID && state.LatestHeadSHA == run.HeadSHA && state.PublishingRunID == run.ID && state.PublishingHeadSHA == run.HeadSHA && state.PublishingLeaseToken == runLeaseToken && state.PublishingLeaseExpiresAt != nil && state.PublishingLeaseExpiresAt.After(now)
	if !owned {
		return reviewworkflow.ErrPublicationLeased
	}
	return nil
}

func decodeReviewPublication(entry model.ReviewPublication) (reviewworkflow.ReviewPublication, error) {
	publication := reviewworkflow.ReviewPublication{
		RunID:       entry.ReviewRunID,
		PayloadHash: entry.PayloadHash,
		Marker:      entry.Marker,
		Finalization: reviewworkflow.ReviewPublicationFinalization{
			Status:           reviewworkflow.RunStatus(entry.IntendedStatus),
			Detail:           entry.FinalDetail,
			AdvanceWatermark: entry.AdvanceWatermark,
		},
		Status:      entry.Status,
		Channel:     entry.Channel,
		ExternalID:  entry.ExternalID,
		PreparedAt:  entry.PreparedAt,
		CompletedAt: entry.CompletedAt,
		ExpiresAt:   entry.ExpiresAt,
	}
	if entry.Status != reviewworkflow.ReviewPublicationStatusPrepared && entry.Status != reviewworkflow.ReviewPublicationStatusCompleted {
		return reviewworkflow.ReviewPublication{}, errors.New("알 수 없는 리뷰 게시 상태입니다")
	}
	if strings.TrimSpace(entry.Marker) == "" || entry.PreparedAt.IsZero() || !entry.ExpiresAt.After(entry.PreparedAt) {
		return reviewworkflow.ReviewPublication{}, errors.New("저장된 리뷰 게시 보존 정보가 올바르지 않습니다")
	}
	if entry.Status == reviewworkflow.ReviewPublicationStatusCompleted {
		if entry.CompletedAt == nil || entry.CompletedAt.Before(entry.PreparedAt) || !entry.ExpiresAt.After(*entry.CompletedAt) || !reviewworkflow.ValidReviewPublicationChannel(entry.Channel, entry.ExternalID) {
			return reviewworkflow.ReviewPublication{}, errors.New("완료된 리뷰 게시 receipt가 올바르지 않습니다")
		}
		if entry.PayloadHash == "" {
			if entry.Channel != reviewworkflow.ReviewPublicationChannelReconciled {
				return reviewworkflow.ReviewPublication{}, errors.New("legacy 리뷰 게시 채널이 올바르지 않습니다")
			}
			return publication, nil
		}
	}
	comments := make([]review.InlineComment, 0)
	if err := json.Unmarshal([]byte(entry.CommentsJSON), &comments); err != nil {
		return reviewworkflow.ReviewPublication{}, fmt.Errorf("저장된 인라인 리뷰 코멘트를 해석하지 못했습니다: %w", err)
	}
	if comments == nil {
		return reviewworkflow.ReviewPublication{}, errors.New("저장된 인라인 리뷰 코멘트가 JSON 배열이 아닙니다")
	}
	payload := reviewworkflow.ReviewPublicationPayload{
		Body:         entry.Body,
		Comments:     comments,
		FallbackBody: entry.FallbackBody,
	}
	if err := payload.Validate(entry.Marker); err != nil {
		return reviewworkflow.ReviewPublication{}, err
	}
	payloadHash, err := payload.Hash()
	if err != nil {
		return reviewworkflow.ReviewPublication{}, err
	}
	if payloadHash != entry.PayloadHash {
		return reviewworkflow.ReviewPublication{}, errors.New("저장된 리뷰 게시 payload hash가 일치하지 않습니다")
	}
	if err := publication.Finalization.Validate(); err != nil {
		return reviewworkflow.ReviewPublication{}, err
	}
	publication.Payload = payload
	return publication, nil
}

func requireCompletedReviewPublication(transaction *gorm.DB, runID uint64, now time.Time) (reviewworkflow.ReviewPublication, error) {
	var publication model.ReviewPublication
	if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("review_run_id = ?", runID).First(&publication).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return reviewworkflow.ReviewPublication{}, reviewworkflow.ErrPublicationIncomplete
		}
		return reviewworkflow.ReviewPublication{}, err
	}
	decoded, err := decodeReviewPublication(publication)
	if err != nil {
		return reviewworkflow.ReviewPublication{}, err
	}
	if decoded.Status != reviewworkflow.ReviewPublicationStatusCompleted {
		return reviewworkflow.ReviewPublication{}, reviewworkflow.ErrPublicationIncomplete
	}
	if !decoded.ExpiresAt.After(now) {
		return reviewworkflow.ReviewPublication{}, reviewworkflow.ErrPublicationExpired
	}
	return decoded, nil
}

func deleteExpiredReviewPublications(transaction *gorm.DB, now time.Time, limit int) (int64, error) {
	var publicationIDs []uint64
	if err := transaction.Table("review_publications AS publications").
		Select("publications.id").
		Joins("LEFT JOIN review_runs AS runs ON runs.id = publications.review_run_id").
		Where("publications.expires_at <= ?", now).
		Where("runs.id IS NULL OR runs.terminal_at IS NOT NULL").
		Order("publications.id ASC").
		Limit(limit).
		Pluck("publications.id", &publicationIDs).Error; err != nil {
		return 0, err
	}
	if len(publicationIDs) == 0 {
		return 0, nil
	}
	deleted := transaction.Where("id IN ? AND expires_at <= ?", publicationIDs, now).Delete(&model.ReviewPublication{})
	return deleted.RowsAffected, deleted.Error
}
