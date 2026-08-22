package dashboard

type ProviderView struct {
	Name          string
	Order         int
	Succeeded     int
	Failed        int
	QuotaBlocked  int
	LastUsedAt    string
	CooldownEnds  string
	LastFailure   string
	LastFailureAt string
}
