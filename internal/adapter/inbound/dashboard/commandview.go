package dashboard

type CommandView struct {
	Repository    string
	RepositoryURL string
	Number        int
	NumberLabel   string
	NumberURL     string
	Invoker       string
	Kind          string
	KindLabel     string
	Allowed       bool
	OccurredAt    string
}
