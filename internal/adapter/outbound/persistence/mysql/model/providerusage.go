package model

import "time"

type ProviderUsage struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement"`
	Provider   string `gorm:"size:40;index"`
	Model      string `gorm:"size:120"`
	Outcome    string `gorm:"size:40;index"`
	Status     int
	OccurredAt time.Time `gorm:"index"`
}

func (ProviderUsage) TableName() string {
	return "provider_usages"
}
