package reviewworkflow

import "time"

const MaxPublicationAttempts = 4

type PublicationAttempt struct {
	Number  int
	Channel string
}

func NextPublicationAttempt(now time.Time, attempt int, retryAfter time.Time) time.Time {
	if attempt < 1 || attempt >= MaxPublicationAttempts {
		if attempt == MaxPublicationAttempts && retryAfter.After(now) {
			return retryAfter
		}
		return time.Time{}
	}
	next := now.Add(10 * time.Minute * time.Duration(1<<(attempt-1)))
	if retryAfter.After(next) {
		return retryAfter
	}
	return next
}
