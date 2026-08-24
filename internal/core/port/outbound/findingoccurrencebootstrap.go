package outbound

import "context"

type FindingOccurrenceBootstrap interface {
	BackfillBatch(ctx context.Context) (bool, error)
}
