package reviewworkflow

import "time"

type UnitSplit struct {
	ParentHash       string
	ParentLeaseToken string
	ParentInputHash  string
	ParentResult     UnitResult
	Children         []Unit
	Assignments      []CoverageAssignment
	RefinedAt        time.Time
}
