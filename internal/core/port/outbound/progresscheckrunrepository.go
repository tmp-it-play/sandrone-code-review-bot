package outbound

import "context"

type ProgressCheckRunRepository interface {
	ProgressCheckRun(ctx context.Context, marker string) (int64, bool, error)
	SaveProgressCheckRun(ctx context.Context, marker string, checkRunID int64) error
}
