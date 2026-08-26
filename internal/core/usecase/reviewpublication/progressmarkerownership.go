package reviewpublication

import (
	"context"
	"time"
)

type ProgressMarkerOwnership interface {
	OwnsProgressMarker(ctx context.Context, runID uint64, marker string) (bool, error)
	ClaimProgressCommentMutation(ctx context.Context, runID uint64, marker string, claimedAt time.Time, leaseExpiresAt time.Time) (string, bool, error)
	CompleteProgressCommentMutation(ctx context.Context, marker string, leaseToken string, completedAt time.Time) error
	FenceProgressCommentMutation(ctx context.Context, marker string, leaseToken string, failedAt time.Time, uncertainUntil time.Time) error
}
