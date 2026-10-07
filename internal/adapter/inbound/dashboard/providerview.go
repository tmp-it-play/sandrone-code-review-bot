package dashboard

type ProviderView struct {
	Name            string
	Order           int
	Succeeded       int
	Failed          int
	QuotaBlocked    int
	InternalBlocked int
	LastUsedAt      string
	CooldownEnds    string
	LastFailure     string
	LastFailureAt   string
}
