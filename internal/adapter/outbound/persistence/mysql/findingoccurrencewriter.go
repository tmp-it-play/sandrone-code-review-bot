package mysql

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maximumPublishedOccurrences = 100

func upsertFindingOccurrences(transaction *gorm.DB, sourceReviewID uint64, sourceRunID uint64, target pullrequest.Target, findings []review.Finding, expiresAt time.Time) error {
	if len(findings) == 0 {
		return nil
	}
	if sourceReviewID == 0 {
		return errors.New("발생 이력의 source review ID가 없습니다")
	}
	now, err := databaseTime(transaction)
	if err != nil {
		return err
	}
	if !expiresAt.After(now) {
		return errors.New("발생 이력 만료 시각이 이미 지났습니다")
	}
	entries, err := findingOccurrenceEntries(sourceReviewID, sourceRunID, target, findings, now, expiresAt)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		var existing model.FindingOccurrence
		err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("owner = ? AND repository = ? AND number = ? AND occurrence_fingerprint = ?", entry.Owner, entry.Repository, entry.Number, entry.OccurrenceFingerprint).
			First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := transaction.Omit(clause.Associations).Create(&entry).Error; err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		active, err := activePublishedFindingOccurrence(transaction, existing.ID, now)
		if err != nil {
			return err
		}
		if active {
			return fmt.Errorf("이미 열린 게시 지적 %s가 있습니다", entry.OccurrenceFingerprint)
		}
		if sameFindingOccurrenceSource(existing, entry) {
			entry.OpenedAt = existing.OpenedAt
			entry.ExpiresAt = existing.ExpiresAt
		}
		updates := map[string]any{
			"current_fingerprint": entry.CurrentFingerprint,
			"root_fingerprint":    entry.RootFingerprint,
			"path":                entry.Path,
			"path_hash":           entry.PathHash,
			"line":                entry.Line,
			"end_line":            entry.EndLine,
			"evidence":            entry.Evidence,
			"status":              findingOccurrenceStatusOpen,
			"source_review_id":    entry.SourceReviewID,
			"source_run_id":       entry.SourceRunID,
			"opened_at":           entry.OpenedAt,
			"resolved_at":         nil,
			"last_checked_at":     nil,
			"expires_at":          entry.ExpiresAt,
			"updated_at":          now,
		}
		if err := transaction.Model(&model.FindingOccurrence{}).Where("id = ?", existing.ID).UpdateColumns(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

func findingOccurrenceEntries(sourceReviewID uint64, sourceRunID uint64, target pullrequest.Target, findings []review.Finding, openedAt time.Time, expiresAt time.Time) ([]model.FindingOccurrence, error) {
	byFingerprint := make(map[string]model.FindingOccurrence)
	for _, finding := range findings {
		occurrences := append([]review.Occurrence{{
			File:     finding.File,
			Line:     finding.Line,
			EndLine:  finding.EndLine,
			Evidence: finding.Evidence,
		}}, finding.Occurrences...)
		for _, occurrence := range occurrences {
			candidate := finding
			candidate.File = occurrence.File
			candidate.Line = occurrence.Line
			candidate.EndLine = occurrence.EndLine
			candidate.Evidence = occurrence.Evidence
			candidate.Occurrences = nil
			fingerprint := review.NewOccurrenceFingerprint(candidate).String()
			if candidate.File == "" || candidate.Evidence == "" || candidate.Line <= 0 || fingerprint == "" {
				continue
			}
			entry := model.FindingOccurrence{
				Owner:                 target.Owner,
				Repository:            target.Repository,
				Number:                target.Number,
				OccurrenceFingerprint: fingerprint,
				CurrentFingerprint:    fingerprint,
				RootFingerprint:       review.NewRootCauseFingerprint(candidate).String(),
				Path:                  candidate.File,
				PathHash:              findingOccurrencePathHash(candidate.File),
				Line:                  candidate.Line,
				EndLine:               candidate.EndLine,
				Evidence:              candidate.Evidence,
				Status:                findingOccurrenceStatusOpen,
				SourceReviewID:        sourceReviewID,
				SourceRunID:           occurrenceSourceRunID(sourceRunID),
				OpenedAt:              openedAt,
				ExpiresAt:             expiresAt,
				CreatedAt:             openedAt,
				UpdatedAt:             openedAt,
			}
			byFingerprint[fingerprint] = entry
		}
	}
	keys := make([]string, 0, len(byFingerprint))
	for fingerprint := range byFingerprint {
		keys = append(keys, fingerprint)
	}
	sort.Strings(keys)
	if len(keys) > maximumPublishedOccurrences {
		return nil, fmt.Errorf("게시 지적 발생 위치가 상한 %d개를 초과했습니다", maximumPublishedOccurrences)
	}
	entries := make([]model.FindingOccurrence, 0, len(keys))
	for _, fingerprint := range keys {
		entries = append(entries, byFingerprint[fingerprint])
	}
	return entries, nil
}

func activePublishedFindingOccurrence(transaction *gorm.DB, occurrenceID uint64, now time.Time) (bool, error) {
	var count int64
	err := transaction.Table("finding_occurrences AS occurrence").
		Joins("JOIN reviews AS source_review ON source_review.id = occurrence.source_review_id").
		Joins("LEFT JOIN review_runs AS source_run ON source_run.id = occurrence.source_run_id").
		Where("occurrence.id = ?", occurrenceID).
		Where("occurrence.status = ? AND occurrence.expires_at > ?", findingOccurrenceStatusOpen, now).
		Where("source_review.outcome IN ?", []string{string(review.OutcomeSucceeded), string(review.OutcomePartial)}).
		Where("occurrence.source_run_id IS NULL OR (source_run.status IN ? AND source_run.expires_at > ?)", []string{string(reviewworkflow.RunStatusComplete), string(reviewworkflow.RunStatusPartial)}, now).
		Count(&count).Error
	return count > 0, err
}

func sameFindingOccurrenceSource(existing model.FindingOccurrence, incoming model.FindingOccurrence) bool {
	if existing.SourceRunID != nil || incoming.SourceRunID != nil {
		return existing.SourceRunID != nil && incoming.SourceRunID != nil && *existing.SourceRunID == *incoming.SourceRunID
	}
	return existing.SourceReviewID == incoming.SourceReviewID
}

func occurrenceSourceRunID(runID uint64) *uint64 {
	if runID == 0 {
		return nil
	}
	return &runID
}
