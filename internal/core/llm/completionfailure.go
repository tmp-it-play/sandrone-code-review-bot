package llm

import (
	"errors"
	"fmt"
	"time"
)

type CompletionFailure struct {
	Attempts        []CompletionAttempt
	Adaptable       bool
	Retryable       bool
	NextAttemptAt   time.Time
	BudgetExhausted bool
}

func (f *CompletionFailure) Error() string {
	if len(f.Attempts) == 0 {
		return "LLM completion failed"
	}
	last := f.Attempts[len(f.Attempts)-1]
	if last.Cause == nil {
		return fmt.Sprintf("LLM completion failed after %d provider decisions", len(f.Attempts))
	}
	return fmt.Sprintf("LLM completion failed after %d provider decisions: %v", len(f.Attempts), last.Cause)
}

func (f *CompletionFailure) Unwrap() []error {
	causes := make([]error, 0, len(f.Attempts))
	for _, attempt := range f.Attempts {
		if attempt.Cause != nil {
			causes = append(causes, attempt.Cause)
		}
	}
	return causes
}

func (f *CompletionFailure) RetryAt() time.Time {
	return f.NextAttemptAt
}

func (f *CompletionFailure) Terminal() bool {
	return f.BudgetExhausted || !f.Retryable
}

func AsCompletionFailure(err error) (*CompletionFailure, bool) {
	var failure *CompletionFailure
	if errors.As(err, &failure) {
		return failure, true
	}
	return nil, false
}
