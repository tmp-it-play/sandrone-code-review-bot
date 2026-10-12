package model

import "time"

type ReviewSummary struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	ReviewRunID    uint64    `gorm:"not null;uniqueIndex"`
	ReviewRun      ReviewRun `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	InputHash      string    `gorm:"size:64;not null"`
	CheckpointJSON string    `gorm:"type:mediumtext;not null"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (ReviewSummary) TableName() string { return "review_summaries" }
