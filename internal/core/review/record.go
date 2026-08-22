package review

import "time"

type Record struct {
	ID            uint64
	Owner         string
	Repository    string
	Number        int
	HeadSHA       string
	Trigger       Trigger
	Outcome       Outcome
	Provider      string
	Model         string
	InlineCount   int
	FallbackCount int
	Detail        string
	StartedAt     time.Time
	FinishedAt    time.Time
}

func (r Record) Duration() time.Duration {
	if r.FinishedAt.IsZero() || r.StartedAt.IsZero() {
		return 0
	}
	return r.FinishedAt.Sub(r.StartedAt)
}
