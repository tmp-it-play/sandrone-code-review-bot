package mysql

import (
	"encoding/json"
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

const findingOccurrenceBootstrapBatchSize = 50
const legacyFindingOccurrenceRetention = 120 * 24 * time.Hour
const maximumFindingOccurrenceRetention = 180 * 24 * time.Hour

type findingOccurrenceBootstrapRow struct {
	FindingID      uint64
	SourceReviewID uint64
	SourceRunID    *uint64
	Owner          string
	Repository     string
	Number         int
	Path           string
	Line           int
	EndLine        int
	Title          string
	Body           string
	Evidence       string
	RootCause      string
	Occurrences    string
	FindingAt      time.Time
	FinishedAt     time.Time
	RunStartedAt   time.Time
	RunTerminalAt  *time.Time
	RunExpiresAt   *time.Time
}

func backfillFindingOccurrencesBatch(database *gorm.DB, afterFindingID uint64, now time.Time) (uint64, bool, error) {
	throughFindingID, found, err := findingOccurrenceBootstrapBoundary(database, afterFindingID)
	if err != nil {
		return 0, false, err
	}
	if !found {
		return afterFindingID, false, nil
	}
	rows, err := findingOccurrenceBootstrapRows(database, afterFindingID, throughFindingID, now)
	if err != nil {
		return 0, false, err
	}
	entries := make([]model.FindingOccurrence, 0, len(rows))
	for _, row := range rows {
		expiresAt := findingOccurrenceBootstrapExpiry(row)
		if !expiresAt.After(now) {
			continue
		}
		openedAt := row.FinishedAt
		if openedAt.IsZero() {
			openedAt = row.FindingAt
		}
		var occurrences []review.Occurrence
		_ = json.Unmarshal([]byte(row.Occurrences), &occurrences)
		if len(occurrences) >= maximumPublishedOccurrences {
			occurrences = occurrences[:maximumPublishedOccurrences-1]
		}
		finding := review.Finding{
			File:        row.Path,
			Line:        row.Line,
			EndLine:     row.EndLine,
			Title:       row.Title,
			Body:        row.Body,
			Evidence:    row.Evidence,
			RootCause:   row.RootCause,
			Occurrences: occurrences,
		}
		target := pullrequest.Target{Owner: row.Owner, Repository: row.Repository, Number: row.Number}
		runID := uint64(0)
		if row.SourceRunID != nil {
			runID = *row.SourceRunID
		}
		findingEntries, entryErr := findingOccurrenceEntries(row.SourceReviewID, runID, target, []review.Finding{finding}, openedAt, expiresAt)
		if entryErr != nil {
			return 0, false, fmt.Errorf("기존 지적 %d의 발생 이력을 변환하지 못했습니다: %w", row.FindingID, entryErr)
		}
		entries = append(entries, findingEntries...)
	}
	if len(entries) > 0 {
		entries, err = reconcileFindingOccurrenceEntries(database, entries)
		if err != nil {
			return 0, false, err
		}
	}
	if len(entries) > 0 {
		if err := database.Omit(clause.Associations).Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "owner"},
				{Name: "repository"},
				{Name: "number"},
				{Name: "occurrence_fingerprint"},
			},
			DoNothing: true,
		}).CreateInBatches(&entries, 100).Error; err != nil {
			return 0, false, fmt.Errorf("기존 지적의 발생 이력을 저장하지 못했습니다: %w", err)
		}
	}
	return throughFindingID, true, nil
}

func reconcileFindingOccurrenceEntries(database *gorm.DB, entries []model.FindingOccurrence) ([]model.FindingOccurrence, error) {
	sort.SliceStable(entries, func(left int, right int) bool {
		leftEntry := entries[left]
		rightEntry := entries[right]
		if leftEntry.Owner != rightEntry.Owner {
			return leftEntry.Owner < rightEntry.Owner
		}
		if leftEntry.Repository != rightEntry.Repository {
			return leftEntry.Repository < rightEntry.Repository
		}
		if leftEntry.Number != rightEntry.Number {
			return leftEntry.Number < rightEntry.Number
		}
		if leftEntry.SourceReviewID != rightEntry.SourceReviewID {
			return leftEntry.SourceReviewID < rightEntry.SourceReviewID
		}
		if leftEntry.Path != rightEntry.Path {
			return leftEntry.Path < rightEntry.Path
		}
		if leftEntry.Line != rightEntry.Line {
			return leftEntry.Line < rightEntry.Line
		}
		return leftEntry.CurrentFingerprint < rightEntry.CurrentFingerprint
	})
	pending := make([]model.FindingOccurrence, 0, len(entries))
	for _, entry := range entries {
		var existingCurrent int64
		if err := database.Model(&model.FindingOccurrence{}).
			Where("owner = ? AND repository = ? AND number = ?", entry.Owner, entry.Repository, entry.Number).
			Where("current_fingerprint = ?", entry.CurrentFingerprint).
			Count(&existingCurrent).Error; err != nil {
			return nil, fmt.Errorf("기존 발생 이력의 현재 fingerprint를 확인하지 못했습니다: %w", err)
		}
		if existingCurrent > 0 {
			continue
		}
		var legacy model.FindingOccurrence
		err := database.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("owner = ? AND repository = ? AND number = ?", entry.Owner, entry.Repository, entry.Number).
			Where("source_review_id = ? AND path_hash = ? AND evidence = ?", entry.SourceReviewID, entry.PathHash, entry.Evidence).
			Where("COALESCE(current_fingerprint, '') = ''").
			Order("id ASC").
			First(&legacy).Error
		if err == nil {
			updates := map[string]any{
				"current_fingerprint": entry.CurrentFingerprint,
				"root_fingerprint":    entry.RootFingerprint,
				"path":                entry.Path,
				"path_hash":           entry.PathHash,
				"line":                entry.Line,
				"end_line":            entry.EndLine,
			}
			if err := database.Model(&model.FindingOccurrence{}).Where("id = ?", legacy.ID).UpdateColumns(updates).Error; err != nil {
				return nil, fmt.Errorf("기존 발생 이력의 fingerprint를 갱신하지 못했습니다: %w", err)
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("기존 발생 이력을 확인하지 못했습니다: %w", err)
		}
		pending = append(pending, entry)
	}
	return pending, nil
}

func findingOccurrenceBootstrapExpiry(row findingOccurrenceBootstrapRow) time.Time {
	expiresAt := row.FinishedAt.Add(legacyFindingOccurrenceRetention)
	if row.RunExpiresAt == nil {
		return expiresAt
	}
	if !row.RunStartedAt.IsZero() {
		retention := legacyFindingOccurrenceRetention
		if row.RunTerminalAt != nil {
			retention = row.RunExpiresAt.Sub(*row.RunTerminalAt)
			if retention < legacyFindingOccurrenceRetention {
				retention = legacyFindingOccurrenceRetention
			}
			if retention > maximumFindingOccurrenceRetention {
				retention = maximumFindingOccurrenceRetention
			}
		}
		expiresAt = row.RunStartedAt.Add(retention)
	}
	if row.RunExpiresAt.Before(expiresAt) {
		return *row.RunExpiresAt
	}
	return expiresAt
}

func findingOccurrenceBootstrapBoundary(database *gorm.DB, afterFindingID uint64) (uint64, bool, error) {
	var findingIDs []uint64
	if err := database.Model(&model.Finding{}).
		Where("id > ?", afterFindingID).
		Order("id ASC").
		Limit(findingOccurrenceBootstrapBatchSize).
		Pluck("id", &findingIDs).Error; err != nil {
		return 0, false, fmt.Errorf("기존 지적 migration 범위를 읽지 못했습니다: %w", err)
	}
	if len(findingIDs) == 0 {
		return afterFindingID, false, nil
	}
	return findingIDs[len(findingIDs)-1], true, nil
}

func findingOccurrenceBootstrapRows(database *gorm.DB, afterFindingID uint64, throughFindingID uint64, now time.Time) ([]findingOccurrenceBootstrapRow, error) {
	var rows []findingOccurrenceBootstrapRow
	err := database.Table("findings AS finding").
		Select("finding.id AS finding_id, finding.review_id AS source_review_id, source_review.review_run_id AS source_run_id, finding.owner, finding.repository, finding.number, finding.path, finding.line, finding.end_line, finding.title, finding.body, finding.evidence, finding.root_cause, finding.occurrences_json AS occurrences, finding.created_at AS finding_at, source_review.finished_at, source_run.started_at AS run_started_at, source_run.terminal_at AS run_terminal_at, source_run.expires_at AS run_expires_at").
		Joins("JOIN reviews AS source_review ON source_review.id = finding.review_id").
		Joins("LEFT JOIN review_runs AS source_run ON source_run.id = source_review.review_run_id").
		Where("finding.id > ? AND finding.id <= ?", afterFindingID, throughFindingID).
		Where("source_review.outcome IN ?", []string{string(review.OutcomeSucceeded), string(review.OutcomePartial)}).
		Where("source_review.review_run_id IS NULL OR source_run.status IN ?", []string{string(reviewworkflow.RunStatusComplete), string(reviewworkflow.RunStatusPartial)}).
		Where("(source_review.review_run_id IS NULL AND source_review.finished_at > ?) OR (source_review.review_run_id IS NOT NULL AND source_run.expires_at > ?)", now.Add(-legacyFindingOccurrenceRetention), now).
		Order("finding.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("기존 지적의 발생 이력을 읽지 못했습니다: %w", err)
	}
	return rows, nil
}
