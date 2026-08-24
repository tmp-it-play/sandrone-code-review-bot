package mysql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
)

const findingOccurrenceStatusOpen = "open"
const findingOccurrenceStatusResolved = "resolved"
const maximumOccurrenceRevalidations = 500
const maximumOccurrencePaths = 4096

func (r *FindingRepository) Fingerprints(ctx context.Context, target pullrequest.Target) (map[string]struct{}, error) {
	var fingerprints []string
	fingerprintExpression := "COALESCE(NULLIF(occurrence.current_fingerprint, ''), occurrence.occurrence_fingerprint)"
	err := activeFindingOccurrences(r.database.WithContext(ctx), target).
		Distinct(fingerprintExpression).
		Order(fingerprintExpression+" ASC").
		Pluck(fingerprintExpression, &fingerprints).Error
	if err != nil {
		return nil, fmt.Errorf("열린 기존 지적을 읽지 못했습니다: %w", err)
	}
	pending, err := r.pendingFindingOccurrenceFingerprints(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("migration 대기 중인 기존 지적을 읽지 못했습니다: %w", err)
	}
	fingerprints = append(fingerprints, pending...)
	known := make(map[string]struct{}, len(fingerprints))
	for _, fingerprint := range fingerprints {
		if fingerprint != "" {
			known[fingerprint] = struct{}{}
		}
	}
	return known, nil
}

func (r *FindingRepository) OpenOccurrences(ctx context.Context, target pullrequest.Target, paths []string, limit int) ([]review.OpenOccurrence, error) {
	paths = boundedOccurrencePaths(paths)
	if len(paths) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > maximumOccurrenceRevalidations {
		limit = maximumOccurrenceRevalidations
	}
	pathHashes := make([]string, 0, len(paths))
	for _, path := range paths {
		pathHashes = append(pathHashes, findingOccurrencePathHash(path))
	}
	var rows []struct {
		OccurrenceFingerprint string
		RootFingerprint       string
		SourceReviewID        uint64
		Path                  string
		Line                  int
		EndLine               int
		Evidence              string
	}
	err := activeFindingOccurrences(r.database.WithContext(ctx), target).
		Select("occurrence.occurrence_fingerprint, occurrence.root_fingerprint, occurrence.source_review_id, occurrence.path, occurrence.line, occurrence.end_line, occurrence.evidence").
		Where("occurrence.path_hash IN ?", pathHashes).
		Order("COALESCE(occurrence.last_checked_at, occurrence.opened_at) ASC").
		Order("occurrence.id ASC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("재검증할 열린 지적을 읽지 못했습니다: %w", err)
	}
	occurrences := make([]review.OpenOccurrence, 0, len(rows))
	for _, row := range rows {
		occurrences = append(occurrences, review.OpenOccurrence{
			ID:             review.Fingerprint(row.OccurrenceFingerprint),
			RootID:         review.Fingerprint(row.RootFingerprint),
			SourceReviewID: row.SourceReviewID,
			Path:           row.Path,
			Line:           row.Line,
			EndLine:        row.EndLine,
			Evidence:       row.Evidence,
		})
	}
	return occurrences, nil
}

func (r *FindingRepository) RevalidateOccurrences(ctx context.Context, target pullrequest.Target, revalidations []review.OccurrenceRevalidation) error {
	revalidations = boundedOccurrenceRevalidations(revalidations)
	if len(revalidations) == 0 {
		return nil
	}
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		now, err := databaseTime(transaction)
		if err != nil {
			return err
		}
		for _, revalidation := range revalidations {
			query := transaction.Model(&model.FindingOccurrence{}).
				Where("owner = ? AND repository = ? AND number = ?", target.Owner, target.Repository, target.Number).
				Where("occurrence_fingerprint = ? AND source_review_id = ?", revalidation.ID.String(), revalidation.SourceReviewID).
				Where("status = ? AND expires_at > ?", findingOccurrenceStatusOpen, now)
			updates := map[string]any{
				"last_checked_at": now,
				"updated_at":      now,
			}
			if revalidation.Resolved {
				updates["status"] = findingOccurrenceStatusResolved
				updates["resolved_at"] = now
			} else if revalidation.CurrentID != "" && revalidation.Line > 0 {
				updates["current_fingerprint"] = revalidation.CurrentID.String()
				updates["line"] = revalidation.Line
				updates["end_line"] = revalidation.EndLine
			}
			if err := query.UpdateColumns(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("열린 지적을 재검증하지 못했습니다: %w", err)
	}
	return nil
}

func activeFindingOccurrences(database *gorm.DB, target pullrequest.Target) *gorm.DB {
	return database.Table("finding_occurrences AS occurrence").
		Joins("JOIN reviews AS source_review ON source_review.id = occurrence.source_review_id").
		Joins("LEFT JOIN review_runs AS source_run ON source_run.id = occurrence.source_run_id").
		Where("occurrence.owner = ? AND occurrence.repository = ? AND occurrence.number = ?", target.Owner, target.Repository, target.Number).
		Where("occurrence.status = ?", findingOccurrenceStatusOpen).
		Where("occurrence.expires_at > CURRENT_TIMESTAMP(6)").
		Where("source_review.outcome IN ?", []string{string(review.OutcomeSucceeded), string(review.OutcomePartial)}).
		Where("occurrence.source_run_id IS NULL OR (source_run.status IN ? AND source_run.expires_at > CURRENT_TIMESTAMP(6))", []string{string(reviewworkflow.RunStatusComplete), string(reviewworkflow.RunStatusPartial)})
}

func boundedOccurrencePaths(paths []string) []string {
	unique := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if path != "" {
			unique[path] = struct{}{}
		}
	}
	paths = make([]string, 0, len(unique))
	for path := range unique {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	if len(paths) > maximumOccurrencePaths {
		paths = paths[:maximumOccurrencePaths]
	}
	return paths
}

func boundedOccurrenceRevalidations(revalidations []review.OccurrenceRevalidation) []review.OccurrenceRevalidation {
	byID := make(map[string]review.OccurrenceRevalidation, len(revalidations))
	for _, revalidation := range revalidations {
		if revalidation.ID == "" || revalidation.SourceReviewID == 0 {
			continue
		}
		key := revalidation.ID.String()
		if existing, found := byID[key]; !found || revalidation.Resolved && !existing.Resolved {
			byID[key] = revalidation
		}
	}
	keys := make([]string, 0, len(byID))
	for key := range byID {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > maximumOccurrenceRevalidations {
		keys = keys[:maximumOccurrenceRevalidations]
	}
	bounded := make([]review.OccurrenceRevalidation, 0, len(keys))
	for _, key := range keys {
		bounded = append(bounded, byID[key])
	}
	return bounded
}

func findingOccurrencePathHash(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:])
}
