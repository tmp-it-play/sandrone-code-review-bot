package model

import "time"

type ProviderUsage struct {
	ID               uint64 `gorm:"primaryKey;autoIncrement"`
	Provider         string `gorm:"size:40;index"`
	Model            string `gorm:"size:120"`
	Role             string `gorm:"size:24;index"`
	Outcome          string `gorm:"size:40;index"`
	Status           int
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	OccurredAt       time.Time `gorm:"index"`
}

func (ProviderUsage) TableName() string {
	return "provider_usages"
}
