package publication

import "time"

type LeaseConflict struct {
	Until time.Time
}

func (e *LeaseConflict) Error() string {
	return ErrLeased.Error()
}

func (e *LeaseConflict) Unwrap() error {
	return ErrLeased
}

func (e *LeaseConflict) RetryAt() time.Time {
	return e.Until
}
