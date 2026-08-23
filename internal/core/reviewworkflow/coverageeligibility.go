package reviewworkflow

type CoverageEligibility string

const (
	CoverageEligibilityEligible   CoverageEligibility = "eligible"
	CoverageEligibilityExcluded   CoverageEligibility = "excluded"
	CoverageEligibilityUnresolved CoverageEligibility = "unresolved"
)
