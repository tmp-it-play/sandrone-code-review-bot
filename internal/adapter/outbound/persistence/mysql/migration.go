package mysql

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"gorm.io/gorm"
)

func Migrate(database *gorm.DB) error {
	return database.Connection(func(connection *gorm.DB) (connectionErr error) {
		connection = connection.Session(&gorm.Session{NewDB: true})
		var acquired sql.NullInt64
		if err := connection.Raw("SELECT GET_LOCK(?, ?)", "sandrone-schema-migration", 600).Scan(&acquired).Error; err != nil {
			return fmt.Errorf("스키마 마이그레이션 lock을 얻지 못했습니다: %w", err)
		}
		if !acquired.Valid || acquired.Int64 != 1 {
			return fmt.Errorf("스키마 마이그레이션 lock 대기 시간이 초과되었습니다")
		}
		defer func() {
			var released sql.NullInt64
			if err := connection.Raw("SELECT RELEASE_LOCK(?)", "sandrone-schema-migration").Scan(&released).Error; err != nil {
				connectionErr = errors.Join(connectionErr, fmt.Errorf("스키마 마이그레이션 lock을 해제하지 못했습니다: %w", err))
			} else if !released.Valid || released.Int64 != 1 {
				connectionErr = errors.Join(connectionErr, fmt.Errorf("스키마 마이그레이션 lock 소유권을 확인하지 못했습니다"))
			}
		}()
		if err := normalizePullRequestStateNumbers(connection); err != nil {
			return err
		}
		if err := connection.AutoMigrate(
			&model.Installation{},
			&model.Repository{},
			&model.PullRequestState{},
			&model.Review{},
			&model.Finding{},
			&model.ProviderUsage{},
			&model.CommandInvocation{},
			&model.ReviewRun{},
			&model.ProgressCommentOwnership{},
			&model.ReviewUnit{},
			&model.ReviewVerification{},
			&model.CoverageItem{},
			&model.ReviewPublication{},
			&model.WebhookDelivery{},
			&model.ReplyPublication{},
			&model.PublicationInvalidation{},
			&model.FindingOccurrence{},
			&model.MigrationCheckpoint{},
		); err != nil {
			return fmt.Errorf("스키마를 반영하지 못했습니다: %w", err)
		}
		if err := backfillPullRequestStateReviewTimes(connection); err != nil {
			return err
		}
		if err := backfillProgressCommentOwnerships(connection); err != nil {
			return err
		}
		if err := backfillReviewVerificationCompletion(connection); err != nil {
			return err
		}
		if err := backfillCoverageInitialUnitHashes(connection); err != nil {
			return err
		}
		if err := removeObsoleteMigrationArtifacts(connection); err != nil {
			return err
		}
		return nil
	})
}

func normalizePullRequestStateNumbers(database *gorm.DB) error {
	if !database.Migrator().HasTable(&model.PullRequestState{}) {
		return nil
	}
	columns := []string{"last_reviewed_run_id", "latest_run_id", "publishing_run_id"}
	for _, column := range columns {
		if !database.Migrator().HasColumn(&model.PullRequestState{}, column) {
			continue
		}
		statement := fmt.Sprintf("UPDATE pull_request_states SET %s = 0 WHERE %s IS NULL", column, column)
		if err := database.Exec(statement).Error; err != nil {
			return fmt.Errorf("pull request 상태 숫자 필드를 정규화하지 못했습니다: %w", err)
		}
	}
	return nil
}

func backfillPullRequestStateReviewTimes(database *gorm.DB) error {
	result := database.Exec("UPDATE pull_request_states SET last_reviewed_at = updated_at WHERE last_reviewed_sha <> '' AND last_reviewed_at IS NULL")
	if result.Error != nil {
		return fmt.Errorf("기존 리뷰 보존 기준 시각을 복원하지 못했습니다: %w", result.Error)
	}
	return nil
}

func backfillProgressCommentOwnerships(database *gorm.DB) error {
	result := database.Exec(`
		INSERT INTO review_progress_comment_ownerships (marker, review_run_id, installation_id, owner, repository, number, refresh_sequence, create_not_before, next_refresh_at, refresh_expires_at, refresh_stop_requested_at, cleanup_lease_token, cleanup_lease_expires_at, created_at, updated_at)
		SELECT selected.progress_marker, selected.id, selected.installation_id, selected.owner, selected.repository, selected.number, 0, NULL, NULL, selected.expires_at, selected.terminal_at, '', NULL, CURRENT_TIMESTAMP(6), CURRENT_TIMESTAMP(6)
		FROM review_runs selected
		INNER JOIN (
			SELECT progress_marker, MAX(id) AS id
			FROM review_runs
			WHERE progress_marker <> ''
			GROUP BY progress_marker
		) latest ON latest.id = selected.id
		ON DUPLICATE KEY UPDATE marker = review_progress_comment_ownerships.marker`)
	if result.Error != nil {
		return fmt.Errorf("진행 코멘트 소유권을 복원하지 못했습니다: %w", result.Error)
	}
	return nil
}
