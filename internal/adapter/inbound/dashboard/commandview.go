package dashboard

type CommandView struct {
	Repository string
	Number     int
	Invoker    string
	Kind       string
	Allowed    bool
	OccurredAt string
}
