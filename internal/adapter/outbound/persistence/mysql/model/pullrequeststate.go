package model

import "time"

type PullRequestState struct {
	ID                       uint64     `gorm:"primaryKey;autoIncrement"`
	Owner                    string     `gorm:"size:100;uniqueIndex:idx_pr_state,priority:1"`
	Repository               string     `gorm:"size:150;uniqueIndex:idx_pr_state,priority:2"`
	Number                   int        `gorm:"uniqueIndex:idx_pr_state,priority:3"`
	LastReviewedSHA          string     `gorm:"size:64"`
	LastReviewedRunID        uint64     `gorm:"not null;default:0"`
	LastReviewedAt           *time.Time `gorm:"index"`
	LatestRunID              uint64     `gorm:"not null;default:0"`
	LatestHeadSHA            string     `gorm:"size:64"`
	LatestObservedAt         *time.Time `gorm:"index"`
	LatestOrderKey           string     `gorm:"size:160;not null;default:''"`
	LatestRunAt              *time.Time `gorm:"index"`
	PublishingRunID          uint64     `gorm:"not null;default:0"`
	PublishingHeadSHA        string     `gorm:"size:64"`
	PublishingLeaseToken     string     `gorm:"size:64"`
	PublishingLeaseExpiresAt *time.Time `gorm:"index"`
	SummaryOperationKey      string     `gorm:"size:64;not null;default:''"`
	SummaryOrderKey          string     `gorm:"size:40;not null;default:''"`
	SummaryObservedAt        *time.Time `gorm:"index"`
	SummaryPublishingKey     string     `gorm:"size:64;not null;default:''"`
	SummaryLeaseToken        string     `gorm:"size:64;not null;default:''"`
	SummaryLeaseExpiresAt    *time.Time `gorm:"index"`
	SummaryCompletedKey      string     `gorm:"size:64;not null;default:''"`
	SummaryCompletedAt       *time.Time `gorm:"index"`
	SummaryExpiresAt         *time.Time `gorm:"index"`
	SummaryExternalCalls     int        `gorm:"not null;default:0"`
	SummaryResultInputHash   string     `gorm:"size:64;not null;default:''"`
	SummaryCanonicalContent  string     `gorm:"type:longtext"`
	SummaryProvider          string     `gorm:"size:100;not null;default:''"`
	SummaryModel             string     `gorm:"size:200;not null;default:''"`
	SummaryModelLabel        string     `gorm:"size:200;not null;default:''"`
	SummaryFinishReason      string     `gorm:"size:64;not null;default:''"`
	SummaryMultipleModels    bool       `gorm:"not null;default:false"`
	SummaryPromptTokens      int        `gorm:"not null;default:0"`
	SummaryCompletionTokens  int        `gorm:"not null;default:0"`
	SummaryTotalTokens       int        `gorm:"not null;default:0"`
	SummaryToolExecutions    int        `gorm:"not null;default:0"`
	SummaryResultCompletedAt *time.Time `gorm:"index"`
	UpdatedAt                time.Time  `gorm:"index"`
}

func (PullRequestState) TableName() string {
	return "pull_request_states"
}
