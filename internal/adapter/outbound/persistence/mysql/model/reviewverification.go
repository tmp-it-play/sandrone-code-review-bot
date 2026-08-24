package model

import "time"

type ReviewVerification struct {
	ID                         uint64    `gorm:"primaryKey;autoIncrement"`
	ReviewRunID                uint64    `gorm:"not null;uniqueIndex"`
	ReviewRun                  ReviewRun `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	InputHash                  string    `gorm:"size:64;not null;index"`
	SupportedOccurrenceIDsJSON string    `gorm:"type:text;not null"`
	Provider                   string    `gorm:"size:40"`
	Model                      string    `gorm:"size:120"`
	MultipleModels             bool      `gorm:"not null;default:false"`
	PromptTokens               int
	CompletionTokens           int
	TotalTokens                int
	ToolExecutions             int
	AttemptCount               int    `gorm:"not null;default:0"`
	Completed                  bool   `gorm:"not null;default:false;index"`
	LastAttemptInputHash       string `gorm:"size:64;not null;default:''"`
	LastAttemptSucceeded       bool   `gorm:"not null;default:false"`
	ResultCompletedAt          *time.Time
	FinishedAt                 *time.Time
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

func (ReviewVerification) TableName() string {
	return "review_verifications"
}
