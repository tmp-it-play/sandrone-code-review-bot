package model

import "time"

type ReviewUnit struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement"`
	ReviewRunID      uint64    `gorm:"not null;uniqueIndex:idx_review_unit_hash,priority:1;index;index:idx_review_unit_parent,priority:1;index:idx_review_unit_order,priority:1"`
	ReviewRun        ReviewRun `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	UnitHash         string    `gorm:"size:64;uniqueIndex:idx_review_unit_hash,priority:2"`
	ParentUnitHash   string    `gorm:"size:64;not null;default:'';index:idx_review_unit_parent,priority:2"`
	InputHash        string    `gorm:"size:64;index"`
	Ordinal          int
	Depth            int    `gorm:"not null;default:0"`
	OrderKey         string `gorm:"size:160;not null;default:'';index:idx_review_unit_order,priority:2"`
	Kind             string `gorm:"size:40"`
	SpecJSON         string `gorm:"type:mediumtext"`
	SplitHash        string `gorm:"size:64;not null;default:''"`
	Status           string `gorm:"size:20;index"`
	AttemptCount     int
	Provider         string `gorm:"size:40"`
	Model            string `gorm:"size:120"`
	MultipleModels   bool   `gorm:"not null;default:false"`
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ToolExecutions   int
	ResultJSON       string `gorm:"type:mediumtext"`
	Reused           bool
	Retryable        bool `gorm:"not null;default:false"`
	RetryAt          *time.Time
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
