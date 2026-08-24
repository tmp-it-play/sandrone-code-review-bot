package reviewpullrequest

import (
	"context"
	"time"
)

func durableCheckpointContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
}
