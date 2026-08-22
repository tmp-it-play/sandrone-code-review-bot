package model

import "time"

type Repository struct {
	ID             uint64 `gorm:"primaryKey;autoIncrement"`
	InstallationID int64  `gorm:"index"`
	Owner          string `gorm:"size:100;uniqueIndex:idx_repository_full,priority:1"`
	Name           string `gorm:"size:150;uniqueIndex:idx_repository_full,priority:2"`
	Private        bool
	UpdatedAt      time.Time
}

func (Repository) TableName() string {
	return "repositories"
}
