package model

import "time"

type WebhookDelivery struct {
	ID              uint64  `gorm:"primaryKey;autoIncrement"`
	DeliveryKey     *string `gorm:"size:160;uniqueIndex"`
	EventType       string  `gorm:"size:100"`
	Payload         []byte  `gorm:"type:longblob"`
	PayloadHash     string  `gorm:"size:64"`
	RequestIdentity string  `gorm:"size:160;uniqueIndex"`
	Status          string  `gorm:"size:20;index:idx_webhook_pending,priority:1;index:idx_webhook_lease,priority:1;index:idx_webhook_terminal,priority:1"`
	Attempts        int
	LeaseToken      string     `gorm:"size:64"`
	LeaseExpiresAt  *time.Time `gorm:"index:idx_webhook_lease,priority:2"`
	AvailableAt     time.Time  `gorm:"index:idx_webhook_pending,priority:2"`
	ReceivedAt      time.Time  `gorm:"index"`
	CompletedAt     *time.Time
	ExpiresAt       *time.Time `gorm:"index:idx_webhook_terminal,priority:2"`
	LastError       string     `gorm:"type:text"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (WebhookDelivery) TableName() string {
	return "webhook_deliveries"
}
