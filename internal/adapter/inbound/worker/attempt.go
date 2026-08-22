package worker

import (
	"context"

	"github.com/hibiken/asynq"
)

func attemptNumber(ctx context.Context) int {
	retried, ok := asynq.GetRetryCount(ctx)
	if !ok {
		return 0
	}
	return retried
}

func isFinalAttempt(ctx context.Context) bool {
	retried, ok := asynq.GetRetryCount(ctx)
	if !ok {
		return true
	}
	maximum, ok := asynq.GetMaxRetry(ctx)
	if !ok {
		return true
	}
	return retried >= maximum
}

func outcomeLabel(err error) string {
	if err != nil {
		return "failed"
	}
	return "succeeded"
}
