package model

import "time"

type ProgressCommentOwnership struct {
	Marker                 string     `gorm:"size:128;primaryKey"`
	ReviewRunID            uint64     `gorm:"not null;index"`
	InstallationID         int64      `gorm:"not null;default:0;index"`
	Owner                  string     `gorm:"size:255;not null;default:''"`
	Repository             string     `gorm:"size:255;not null;default:''"`
	Number                 int        `gorm:"not null;default:0"`
	HeadSHA                string     `gorm:"size:64;not null;default:''"`
	CheckRunID             int64      `gorm:"not null;default:0"`
	ProgressMessageTheme   string     `gorm:"size:32;not null;default:programming"`
	RefreshSequence        uint64     `gorm:"not null;default:0"`
	CreateNotBefore        *time.Time `gorm:"index"`
	NextRefreshAt          *time.Time `gorm:"index"`
	RefreshExpiresAt       *time.Time `gorm:"index"`
	RefreshStopRequestedAt *time.Time `gorm:"index"`
	CleanupLeaseToken      string     `gorm:"size:64;not null;default:''"`
	CleanupLeaseExpiresAt  *time.Time `gorm:"index"`
	CreatedAt              time.Time  `gorm:"not null"`
	UpdatedAt              time.Time  `gorm:"not null"`
}

func (ProgressCommentOwnership) TableName() string {
	return "review_progress_comment_ownerships"
}
