package job

import (
	"fmt"
	"time"
)

type RetryAtError struct {
	At    time.Time
	Cause error
}

func (e *RetryAtError) Error() string {
	return fmt.Sprintf("%v; retry at %s", e.Cause, e.At.UTC().Format(time.RFC3339))
}

func (e *RetryAtError) Unwrap() error {
	return e.Cause
}

func (e *RetryAtError) RetryAt() time.Time {
	return e.At
}
