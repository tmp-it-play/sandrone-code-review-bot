package model

import "time"

type PublicationInvalidation struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement"`
	ReviewRunID    uint64     `gorm:"not null;uniqueIndex"`
	InstallationID int64      `gorm:"not null"`
	Owner          string     `gorm:"size:100;not null;index:idx_publication_invalidation_target,priority:1"`
	Repository     string     `gorm:"size:150;not null;index:idx_publication_invalidation_target,priority:2"`
	Number         int        `gorm:"not null;index:idx_publication_invalidation_target,priority:3"`
	Marker         string     `gorm:"size:200;not null"`
	Reason         string     `gorm:"size:2000;not null"`
	Attempts       int        `gorm:"not null;default:0"`
	LastError      string     `gorm:"size:1000;not null;default:''"`
	NextAttemptAt  time.Time  `gorm:"not null;index:idx_publication_invalidation_claim,priority:2"`
	LeaseToken     string     `gorm:"size:64;not null;default:''"`
	LeaseExpiresAt *time.Time `gorm:"index"`
	ResolvedAt     *time.Time `gorm:"index:idx_publication_invalidation_claim,priority:1"`
	ExpiresAt      time.Time  `gorm:"not null;index"`
	CreatedAt      time.Time  `gorm:"not null"`
	UpdatedAt      time.Time  `gorm:"not null"`
}

func (PublicationInvalidation) TableName() string {
	return "review_publication_invalidations"
}
