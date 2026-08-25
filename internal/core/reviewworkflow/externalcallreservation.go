package reviewworkflow

import "time"

type ExternalCallReservation struct {
	RunID              uint64
	RunLeaseToken      string
	UnitHash           string
	UnitLeaseToken     string
	Limit              int
	HeartbeatAt        time.Time
	RunLeaseExpiresAt  time.Time
	UnitLeaseExpiresAt time.Time
}

func (r ExternalCallReservation) HasUnit() bool {
	return r.UnitHash != "" || r.UnitLeaseToken != "" || !r.UnitLeaseExpiresAt.IsZero()
}
