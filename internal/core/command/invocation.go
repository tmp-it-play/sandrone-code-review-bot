package command

import "time"

type Invocation struct {
	Owner      string
	Repository string
	Number     int
	Invoker    string
	Kind       Kind
	Allowed    bool
	Detail     string
	OccurredAt time.Time
}
