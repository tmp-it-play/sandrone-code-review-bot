package reviewworkflow

import "time"

type CoverageItem struct {
	ID               uint64
	RunID            uint64
	UnitID           *uint64
	UnitHash         string
	Key              string
	Kind             CoverageKind
	Path             string
	PreviousPath     string
	FileStatus       string
	HunkHash         string
	DuplicateOrdinal int
	OldStart         int
	OldCount         int
	NewStart         int
	NewCount         int
	Eligibility      CoverageEligibility
	Status           CoverageStatus
	Reason           string
	ReviewedAt       *time.Time
}
