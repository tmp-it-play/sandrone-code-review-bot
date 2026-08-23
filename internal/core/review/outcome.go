package review

type Outcome string

const (
	OutcomeSucceeded   Outcome = "succeeded"
	OutcomePartial     Outcome = "partial"
	OutcomeSkipped     Outcome = "skipped"
	OutcomeFailed      Outcome = "failed"
	OutcomeUnavailable Outcome = "unavailable"
	OutcomeSuperseded  Outcome = "superseded"
)
