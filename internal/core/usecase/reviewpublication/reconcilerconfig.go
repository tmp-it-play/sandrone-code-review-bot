package reviewpublication

import "time"

type ReconcilerConfig struct {
	Retention         time.Duration
	RunLease          time.Duration
	PublicationLease  time.Duration
	OrphanAfter       time.Duration
	ReconcileTimeout  time.Duration
	BatchSize         int
	InvalidationLimit int
}
