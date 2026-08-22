package model

import "time"

type Finding struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	ReviewID    uint64 `gorm:"index"`
	Owner       string `gorm:"size:100;index:idx_finding_target,priority:1"`
	Repository  string `gorm:"size:150;index:idx_finding_target,priority:2"`
	Number      int    `gorm:"index:idx_finding_target,priority:3"`
	Fingerprint string `gorm:"size:64;index"`
	Path        string `gorm:"size:500"`
	Line        int
	Severity    string `gorm:"size:20"`
	Title       string `gorm:"size:500"`
	Body        string `gorm:"type:text"`
	Suggestion  string `gorm:"type:text"`
	Placement   string `gorm:"size:20"`
	CreatedAt   time.Time
}

func (Finding) TableName() string {
	return "findings"
}
