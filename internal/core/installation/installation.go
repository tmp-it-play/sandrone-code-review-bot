package installation

import "time"

type Installation struct {
	ID          int64
	Account     string
	AccountType string
	Selection   string
	InstalledAt time.Time
}
