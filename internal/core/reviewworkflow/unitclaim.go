package reviewworkflow

type UnitClaim struct {
	LeaseToken string
	Completed  bool
	Reused     bool
	Result     UnitResult
}
