package model

import "time"

type ReplyPublication struct {
	ID             uint64 `gorm:"primaryKey;autoIncrement"`
	OperationKey   string `gorm:"size:64;uniqueIndex"`
	Owner          string `gorm:"size:100;index:idx_reply_publication_target,priority:1"`
	Repository     string `gorm:"size:150;index:idx_reply_publication_target,priority:2"`
	Number         int    `gorm:"index:idx_reply_publication_target,priority:3"`
	CommentID      int64
	InThread       bool
	LeaseToken     string     `gorm:"size:64;not null;default:''"`
	LeaseExpiresAt *time.Time `gorm:"index"`
	CompletedAt    *time.Time `gorm:"index"`
	ExpiresAt      time.Time  `gorm:"index"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (ReplyPublication) TableName() string {
	return "reply_publications"
}
