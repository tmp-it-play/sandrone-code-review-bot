package bootstrap

import (
	"errors"
	"time"
)

func retryDelayFor(attempt int, cause error, now time.Time) time.Duration {
	var scheduled interface{ RetryAt() time.Time }
	if errors.As(cause, &scheduled) {
		delay := scheduled.RetryAt().Sub(now)
		if delay > 0 {
			return delay
		}
	}
	return retryDelay(attempt)
}

func retryDelay(attempt int) time.Duration {
	delays := []time.Duration{
		1 * time.Minute,
		5 * time.Minute,
		15 * time.Minute,
		30 * time.Minute,
		1 * time.Hour,
		2 * time.Hour,
	}
	if attempt < 1 {
		attempt = 1
	}
	if attempt > len(delays) {
		attempt = len(delays)
	}
	return delays[attempt-1]
}
