package usage

import "time"

type Event struct {
	Provider   string
	Model      string
	Outcome    string
	Status     int
	OccurredAt time.Time
}
