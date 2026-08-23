package model

import "time"

type CoverageItem struct {
	ID               uint64      `gorm:"primaryKey;autoIncrement"`
	ReviewRunID      uint64      `gorm:"not null;uniqueIndex:idx_coverage_item_key,priority:1;index:idx_coverage_state,priority:1"`
	ReviewRun        ReviewRun   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ReviewUnitID     *uint64     `gorm:"index"`
	ReviewUnit       *ReviewUnit `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	CoverageKey      string      `gorm:"size:64;uniqueIndex:idx_coverage_item_key,priority:2"`
	Kind             string      `gorm:"size:30"`
	Path             string      `gorm:"size:500"`
	PreviousPath     string      `gorm:"size:500"`
	FileStatus       string      `gorm:"size:30"`
	HunkHash         string      `gorm:"size:64"`
	DuplicateOrdinal int
	OldStart         int
	OldCount         int
	NewStart         int
	NewCount         int
	Eligibility      string `gorm:"size:20;index:idx_coverage_state,priority:2"`
	Status           string `gorm:"size:20;index:idx_coverage_state,priority:3"`
	Reason           string `gorm:"size:80"`
	ReviewedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (CoverageItem) TableName() string {
	return "coverage_items"
}
