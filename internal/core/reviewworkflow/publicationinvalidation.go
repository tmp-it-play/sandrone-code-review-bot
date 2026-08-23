package reviewworkflow

import "time"

type PublicationInvalidation struct {
	ID             uint64
	RunID          uint64
	InstallationID int64
	Owner          string
	Repository     string
	Number         int
	Marker         string
	Reason         string
	Attempts       int
	LastError      string
	NextAttemptAt  time.Time
	LeaseToken     string
	LeaseExpiresAt *time.Time
	ResolvedAt     *time.Time
	ExpiresAt      time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
