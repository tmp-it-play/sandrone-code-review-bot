package review

type Outcome string

const (
	OutcomeSucceeded   Outcome = "succeeded"
	OutcomeSkipped     Outcome = "skipped"
	OutcomeFailed      Outcome = "failed"
	OutcomeUnavailable Outcome = "unavailable"
)
