package usage

import "time"

type Snapshot struct {
	Provider                              string
	Succeeded                             int
	Failed                                int
	QuotaBlocked                          int
	InternalBlocked                       int
	LastUsedAt                            time.Time
	CooldownEnds                          time.Time
	LastFailureKind                       string
	LastFailureStatus                     int
	LastFailureProviderErrorCode          string
	LastFailureRequestElapsedMilliseconds int64
	LastFailureAt                         time.Time
}
