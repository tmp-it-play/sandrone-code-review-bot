package github

import "time"

type cachedToken struct {
	value     string
	expiresAt time.Time
}

func (t cachedToken) valid(now time.Time) bool {
	return t.value != "" && now.Before(t.expiresAt.Add(-time.Minute))
}
