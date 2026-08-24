package webhookrecovery

import "time"

type Delivery struct {
	ID          int64
	GUID        string
	Event       string
	Status      string
	StatusCode  int
	DeliveredAt time.Time
}
