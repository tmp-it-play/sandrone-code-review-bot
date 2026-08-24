package worker

import "time"

type JobMetrics interface {
	ObserveJob(kind string, outcome string, elapsed time.Duration)
}
