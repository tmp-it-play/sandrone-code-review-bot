package dashboard

type ReviewView struct {
	ID         uint64
	Repository string
	Number     int
	Trigger    string
	Outcome    string
	Provider   string
	Model      string
	Inline     int
	Fallback   int
	Duration   string
	StartedAt  string
	Detail     string
}
