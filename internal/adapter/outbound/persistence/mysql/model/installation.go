package model

import "time"

type Installation struct {
	ID          int64  `gorm:"primaryKey"`
	Account     string `gorm:"size:150;index"`
	AccountType string `gorm:"size:40"`
	Selection   string `gorm:"size:40"`
	InstalledAt time.Time
	UpdatedAt   time.Time
}

func (Installation) TableName() string {
	return "installations"
}
