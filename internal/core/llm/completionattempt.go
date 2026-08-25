package llm

import "time"

type CompletionAttempt struct {
	Provider  string
	Model     string
	Kind      CompletionAttemptKind
	RetryAt   time.Time
	Invoked   bool
	Adaptable bool
	Cause     error
}
