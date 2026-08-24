package reviewworkflow

import "time"

type ReviewPublication struct {
	RunID        uint64
	PayloadHash  string
	Marker       string
	Payload      ReviewPublicationPayload
	Finalization ReviewPublicationFinalization
	Status       string
	Channel      string
	ExternalID   int64
	PreparedAt   time.Time
	CompletedAt  *time.Time
	ExpiresAt    time.Time
}
