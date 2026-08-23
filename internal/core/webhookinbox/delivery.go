package webhookinbox

import "time"

type Delivery struct {
	ID              uint64
	Key             string
	EventType       string
	Payload         []byte
	PayloadHash     string
	RequestIdentity string
	Status          Status
	Attempts        int
	LeaseToken      string
	LeaseExpiresAt  *time.Time
	AvailableAt     time.Time
	ReceivedAt      time.Time
	CompletedAt     *time.Time
	ExpiresAt       *time.Time
	LastError       string
}
