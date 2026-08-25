package worker

import (
	"errors"
	"time"
)

func outcomeLabel(err error) string {
	if err == nil {
		return "succeeded"
	}
	var scheduled interface{ RetryAt() time.Time }
	if errors.As(err, &scheduled) && !scheduled.RetryAt().IsZero() {
		return "retrying"
	}
	return "failed"
}
