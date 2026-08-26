package publication

import "time"

type RetryAtError struct {
	At    time.Time
	Cause error
}

func (e *RetryAtError) Error() string {
	if e == nil || e.Cause == nil {
		return "GitHub 요청을 나중에 다시 시도해야 합니다"
	}
	return e.Cause.Error()
}

func (e *RetryAtError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *RetryAtError) RetryAt() time.Time {
	if e == nil {
		return time.Time{}
	}
	return e.At
}
