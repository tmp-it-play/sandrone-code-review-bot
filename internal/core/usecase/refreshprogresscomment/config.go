package refreshprogresscomment

import "time"

type Config struct {
	Lease       time.Duration
	Timeout     time.Duration
	Limit       int
	Concurrency int
}
