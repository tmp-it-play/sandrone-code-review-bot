package mysql

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func NewConnection(dsn string) (*gorm.DB, error) {
	database, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger:                 logger.Default.LogMode(logger.Silent),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		return nil, fmt.Errorf("MySQL에 연결하지 못했습니다: %w", err)
	}
	pool, err := database.DB()
	if err != nil {
		return nil, fmt.Errorf("커넥션 풀을 얻지 못했습니다: %w", err)
	}
	pool.SetMaxOpenConns(20)
	pool.SetMaxIdleConns(10)
	pool.SetConnMaxLifetime(time.Hour)
	return database, nil
}

func Migrate(database *gorm.DB) error {
	return database.Connection(func(connection *gorm.DB) (connectionErr error) {
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
			&model.ReviewUnit{},
			&model.CoverageItem{},
			&model.WebhookDelivery{},
			&model.ReplyPublication{},
			&model.PublicationInvalidation{},
		); err != nil {
			return fmt.Errorf("스키마를 반영하지 못했습니다: %w", err)
		}
		if err := backfillPullRequestStateReviewTimes(connection); err != nil {
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
