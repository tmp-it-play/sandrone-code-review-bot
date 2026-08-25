package reviewworkflow

type UnitClaim struct {
	LeaseToken string
	Completed  bool
	Waiting    bool
	Reused     bool
	Result     UnitResult
}
