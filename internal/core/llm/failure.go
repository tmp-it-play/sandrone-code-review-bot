package llm

import (
	"errors"
	"fmt"
	"time"
)

type Failure struct {
	Provider          string
	Kind              FailureKind
	Status            int
	ProviderErrorCode string
	RetryAfter        time.Duration
	Elapsed           time.Duration
	Cause             error
}

func (f *Failure) Error() string {
	return fmt.Sprintf("%s: %s (status %d): %v", f.Provider, f.Kind, f.Status, f.Cause)
}

func (f *Failure) Unwrap() error {
	return f.Cause
}

func AsFailure(err error) (*Failure, bool) {
	var failure *Failure
	if errors.As(err, &failure) {
		return failure, true
	}
	return nil, false
}
