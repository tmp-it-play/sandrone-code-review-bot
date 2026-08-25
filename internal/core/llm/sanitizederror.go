package llm

type SanitizedError struct {
	Message string
	Cause   error
}

func (e *SanitizedError) Error() string {
	return e.Message
}

func (e *SanitizedError) Unwrap() error {
	return e.Cause
}
