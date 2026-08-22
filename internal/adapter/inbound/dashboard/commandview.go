package dashboard

type CommandView struct {
	Repository string
	Number     int
	Invoker    string
	Kind       string
	KindLabel  string
	Allowed    bool
	OccurredAt string
}
