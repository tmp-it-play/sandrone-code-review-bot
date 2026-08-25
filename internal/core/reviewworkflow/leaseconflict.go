package reviewworkflow

import "time"

type LeaseConflict struct {
	Cause error
	Until time.Time
}

func (e *LeaseConflict) Error() string {
	return e.Cause.Error()
}

func (e *LeaseConflict) Unwrap() error {
	return e.Cause
}

func (e *LeaseConflict) RetryAt() time.Time {
	return e.Until
}
