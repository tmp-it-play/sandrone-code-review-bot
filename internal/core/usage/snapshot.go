package usage

import "time"

type Snapshot struct {
	Provider          string
	Succeeded         int
	Failed            int
	QuotaBlocked      int
	LastUsedAt        time.Time
	CooldownEnds      time.Time
	LastFailureKind   string
	LastFailureStatus int
	LastFailureAt     time.Time
}
