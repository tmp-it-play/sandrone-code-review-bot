package reviewworkflow

type coverageClassification struct {
	eligibility CoverageEligibility
	status      CoverageStatus
	reason      string
}
