package model

import "time"

type ReviewRun struct {
	ID                 uint64 `gorm:"primaryKey;autoIncrement"`
	RunKey             string `gorm:"size:64;uniqueIndex"`
	InstallationID     int64
	Owner              string    `gorm:"size:100;index:idx_review_run_target,priority:1"`
	Repository         string    `gorm:"size:150;index:idx_review_run_target,priority:2"`
	Number             int       `gorm:"index:idx_review_run_target,priority:3"`
	BaseSHA            string    `gorm:"size:64"`
	HeadSHA            string    `gorm:"size:64;index"`
	ConfigHash         string    `gorm:"size:64"`
	PromptVersion      string    `gorm:"size:40"`
	ModelPolicyHash    string    `gorm:"size:64"`
	RequestIdentity    string    `gorm:"size:160"`
	Trigger            string    `gorm:"size:40"`
	Status             string    `gorm:"size:20;index"`
	SnapshotObservedAt time.Time `gorm:"index"`
	SnapshotOrderKey   string    `gorm:"size:160;not null;default:''"`
	TotalCoverage      int
	ReviewedCoverage   int
	FailedCoverage     int
	DeferredCoverage   int
	SkippedCoverage    int
	ExternalCalls      int    `gorm:"not null;default:0"`
	ErrorSummary       string `gorm:"size:1000"`
	StartedAt          time.Time
	HeartbeatAt        time.Time  `gorm:"index"`
	TerminalAt         *time.Time `gorm:"index"`
	ExpiresAt          *time.Time `gorm:"index"`
	LeaseToken         string     `gorm:"size:64"`
	LeaseExpiresAt     *time.Time `gorm:"index"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (ReviewRun) TableName() string {
	return "review_runs"
}
