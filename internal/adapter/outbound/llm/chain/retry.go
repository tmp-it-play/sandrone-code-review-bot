package chain

import (
	"math/rand/v2"
	"time"
)

const transientRetries = 1

const maximumImmediateRetryElapsed = 10 * time.Second

func retryPause(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	base := 800 * time.Millisecond * time.Duration(1<<(attempt-1))
	jitter := time.Duration(rand.N(400)) * time.Millisecond
	return base + jitter
}
