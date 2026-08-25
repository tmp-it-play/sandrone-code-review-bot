package publication

type Claim struct {
	LeaseToken    string
	ExternalCalls int
	Completed     bool
	Superseded    bool
}
