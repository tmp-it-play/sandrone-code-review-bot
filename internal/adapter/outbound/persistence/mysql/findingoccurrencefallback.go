package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
)

const maximumPendingOccurrenceFindings = 5000

type pendingFindingOccurrenceRow struct {
	Path        string
	Line        int
	EndLine     int
	Title       string
	Body        string
	Evidence    string
	RootCause   string
	Occurrences string
}

func (r *FindingRepository) pendingFindingOccurrenceFingerprints(ctx context.Context, target pullrequest.Target) ([]string, error) {
	database := r.database.WithContext(ctx)
	checkpoint := model.MigrationCheckpoint{}
	err := database.Where("name = ?", findingOccurrenceBootstrapCheckpoint).First(&checkpoint).Error
	if err == nil && checkpoint.CompletedAt != nil {
		return nil, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	cursor := checkpoint.Cursor
	now, err := databaseTime(database)
	if err != nil {
		return nil, err
	}
	var rows []pendingFindingOccurrenceRow
	err = database.Table("findings AS finding").
		Select("finding.path, finding.line, finding.end_line, finding.title, finding.body, finding.evidence, finding.root_cause, finding.occurrences_json AS occurrences").
		Joins("JOIN reviews AS source_review ON source_review.id = finding.review_id").
		Joins("LEFT JOIN review_runs AS source_run ON source_run.id = source_review.review_run_id").
		Where("finding.id > ?", cursor).
		Where("finding.owner = ? AND finding.repository = ? AND finding.number = ?", target.Owner, target.Repository, target.Number).
		Where("source_review.outcome IN ?", []string{string(review.OutcomeSucceeded), string(review.OutcomePartial)}).
		Where("source_review.review_run_id IS NULL OR (source_run.status IN ? AND source_run.expires_at > CURRENT_TIMESTAMP(6))", []string{string(reviewworkflow.RunStatusComplete), string(reviewworkflow.RunStatusPartial)}).
		Where("source_review.review_run_id IS NOT NULL OR source_review.finished_at > ?", now.Add(-legacyFindingOccurrenceRetention)).
		Order("finding.id DESC").
		Limit(maximumPendingOccurrenceFindings).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	known := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		var occurrences []review.Occurrence
		_ = json.Unmarshal([]byte(row.Occurrences), &occurrences)
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
		candidates := append([]review.Occurrence{{
			File:     finding.File,
			Line:     finding.Line,
			EndLine:  finding.EndLine,
			Evidence: finding.Evidence,
		}}, finding.Occurrences...)
		for _, occurrence := range candidates {
			if strings.TrimSpace(occurrence.File) == "" || occurrence.Line <= 0 || occurrence.Evidence == "" {
				continue
			}
			candidate := finding
			candidate.File = occurrence.File
			candidate.Line = occurrence.Line
			candidate.EndLine = occurrence.EndLine
			candidate.Evidence = occurrence.Evidence
			candidate.Occurrences = nil
			known[review.NewOccurrenceFingerprint(candidate).String()] = struct{}{}
		}
	}
	fingerprints := make([]string, 0, len(known))
	for fingerprint := range known {
		if fingerprint == "" {
			return nil, fmt.Errorf("기존 지적의 occurrence fingerprint를 만들지 못했습니다")
		}
		fingerprints = append(fingerprints, fingerprint)
	}
	sort.Strings(fingerprints)
	return fingerprints, nil
}
