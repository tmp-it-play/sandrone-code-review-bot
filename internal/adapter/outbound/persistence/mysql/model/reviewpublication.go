package model

import "time"

type ReviewPublication struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement"`
	ReviewRunID      uint64    `gorm:"not null;uniqueIndex"`
	ReviewRun        ReviewRun `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	PayloadHash      string    `gorm:"size:64;not null"`
	Marker           string    `gorm:"size:160;not null"`
	Body             string    `gorm:"type:longtext;not null"`
	CommentsJSON     string    `gorm:"type:longtext;not null"`
	FallbackBody     string    `gorm:"type:longtext;not null"`
	IntendedStatus   string    `gorm:"size:20;not null"`
	FinalDetail      string    `gorm:"type:text;not null"`
	AdvanceWatermark bool      `gorm:"not null;default:false"`
	Status           string    `gorm:"size:20;not null;index"`
	Channel          string    `gorm:"size:24"`
	ExternalID       int64
	PreparedAt       time.Time
	CompletedAt      *time.Time
	ExpiresAt        time.Time `gorm:"not null;index"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (ReviewPublication) TableName() string {
	return "review_publications"
}
