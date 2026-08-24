package model

import "time"

type MigrationCheckpoint struct {
	Name        string `gorm:"size:100;primaryKey"`
	Cursor      uint64 `gorm:"not null;default:0"`
	CompletedAt *time.Time
	UpdatedAt   time.Time `gorm:"not null"`
}

func (MigrationCheckpoint) TableName() string {
	return "migration_checkpoints"
}
