package model

import "time"

type PullRequestState struct {
	ID              uint64 `gorm:"primaryKey;autoIncrement"`
	Owner           string `gorm:"size:100;uniqueIndex:idx_pr_state,priority:1"`
	Repository      string `gorm:"size:150;uniqueIndex:idx_pr_state,priority:2"`
	Number          int    `gorm:"uniqueIndex:idx_pr_state,priority:3"`
	LastReviewedSHA string `gorm:"size:64"`
	UpdatedAt       time.Time
}

func (PullRequestState) TableName() string {
	return "pull_request_states"
}
