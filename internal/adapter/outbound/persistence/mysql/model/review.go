package model

import "time"

type Review struct {
	ID            uint64  `gorm:"primaryKey;autoIncrement"`
	ReviewRunID   *uint64 `gorm:"uniqueIndex"`
	Owner         string  `gorm:"size:100;index:idx_review_target,priority:1"`
	Repository    string  `gorm:"size:150;index:idx_review_target,priority:2"`
	Number        int     `gorm:"index:idx_review_target,priority:3"`
	HeadSHA       string  `gorm:"size:64"`
	Trigger       string  `gorm:"size:40"`
	Outcome       string  `gorm:"size:20;index"`
	Provider      string  `gorm:"size:40"`
	Model         string  `gorm:"size:120"`
	InlineCount   int
	FallbackCount int
	Detail        string `gorm:"type:text"`
	StartedAt     time.Time
	FinishedAt    time.Time `gorm:"index"`
}

func (Review) TableName() string {
	return "reviews"
}
