package mysql

import (
	"database/sql"
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func NewConnection(dsn string) (*gorm.DB, *sql.DB, error) {
	database, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger:                 logger.Default.LogMode(logger.Silent),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("MySQL에 연결하지 못했습니다: %w", err)
	}
	pool, err := database.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("커넥션 풀을 얻지 못했습니다: %w", err)
	}
	pool.SetMaxOpenConns(20)
	pool.SetMaxIdleConns(10)
	pool.SetConnMaxLifetime(time.Hour)
	return database, pool, nil
}
