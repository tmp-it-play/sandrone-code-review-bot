package webhook

import "time"

type Retention time.Duration

func (r Retention) Duration() time.Duration {
	return time.Duration(r)
}
