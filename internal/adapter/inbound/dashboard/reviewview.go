package dashboard

type ReviewView struct {
	ID            uint64
	Repository    string
	RepositoryURL string
	Number        int
	NumberLabel   string
	NumberURL     string
	Trigger       string
	TriggerLabel  string
	Outcome       string
	OutcomeLabel  string
	Provider      string
	Model         string
	Inline        int
	Fallback      int
	Duration      string
	StartedAt     string
	Detail        string
}
