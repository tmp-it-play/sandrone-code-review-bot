package mysql

import (
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
	err := database.AutoMigrate(
		&model.Installation{},
		&model.Repository{},
		&model.PullRequestState{},
		&model.Review{},
		&model.Finding{},
		&model.ProviderUsage{},
		&model.CommandInvocation{},
	)
	if err != nil {
		return fmt.Errorf("스키마를 반영하지 못했습니다: %w", err)
	}
	return nil
}
