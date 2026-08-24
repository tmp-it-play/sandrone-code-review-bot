package replythread

import (
	"context"
	"time"
)

func completionCheckpointContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
}
