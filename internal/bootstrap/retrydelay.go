package bootstrap

import (
	"errors"
	"hash/fnv"
	"time"

	"github.com/hibiken/asynq"
	"github.com/it-play/sandrone-code-review-bot/internal/core/backoff"
)

var taskRetryBackoff = backoff.Policy{Initial: time.Minute, Maximum: 2 * time.Hour}

func retryDelayFor(attempt int, cause error, now time.Time) time.Duration {
	return retryDelayForSeed(attempt, cause, now, 0)
}

func retryDelayForTask(attempt int, cause error, now time.Time, task *asynq.Task) time.Duration {
	return retryDelayForSeed(attempt, cause, now, taskRetrySeed(task))
}

func retryDelayForSeed(attempt int, cause error, now time.Time, seed uint64) time.Duration {
	var scheduled interface{ RetryAt() time.Time }
	if errors.As(cause, &scheduled) {
		delay := scheduled.RetryAt().Sub(now)
		if delay > 0 {
			return delay
		}
	}
	return taskRetryBackoff.Delay(attempt, seed)
}

func retryDelay(attempt int) time.Duration {
	return taskRetryBackoff.Delay(attempt, 0)
}

func taskRetrySeed(task *asynq.Task) uint64 {
	if task == nil {
		return 0
	}
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(task.Type()))
	_, _ = hash.Write(task.Payload())
	return hash.Sum64()
}
