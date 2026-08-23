package model

import "time"

type ReviewUnit struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement"`
	ReviewRunID      uint64    `gorm:"not null;uniqueIndex:idx_review_unit_hash,priority:1;index"`
	ReviewRun        ReviewRun `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	UnitHash         string    `gorm:"size:64;uniqueIndex:idx_review_unit_hash,priority:2"`
	Ordinal          int
	Kind             string `gorm:"size:40"`
	Status           string `gorm:"size:20;index"`
	AttemptCount     int
	Provider         string `gorm:"size:40"`
	Model            string `gorm:"size:120"`
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ErrorSummary     string `gorm:"size:1000"`
	StartedAt        *time.Time
	FinishedAt       *time.Time
	HeartbeatAt      *time.Time
	LeaseToken       string     `gorm:"size:64"`
	LeaseExpiresAt   *time.Time `gorm:"index"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (ReviewUnit) TableName() string {
	return "review_units"
}
