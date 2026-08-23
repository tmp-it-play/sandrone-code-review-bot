package command

import "time"

type Invocation struct {
	Key        string
	Owner      string
	Repository string
	Number     int
	Invoker    string
	Kind       Kind
	Allowed    bool
	Detail     string
	OccurredAt time.Time
}
