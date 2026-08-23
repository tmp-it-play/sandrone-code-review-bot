package publication

type Claim struct {
	LeaseToken string
	Completed  bool
	Superseded bool
}
