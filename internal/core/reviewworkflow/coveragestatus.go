package reviewworkflow

type CoverageStatus string

const (
	CoverageStatusIndexed      CoverageStatus = "indexed"
	CoverageStatusPlanned      CoverageStatus = "planned"
	CoverageStatusReviewed     CoverageStatus = "reviewed"
	CoverageStatusDeepReviewed CoverageStatus = "deep_reviewed"
	CoverageStatusFailed       CoverageStatus = "failed"
	CoverageStatusDeferred     CoverageStatus = "deferred"
	CoverageStatusSkipped      CoverageStatus = "skipped"
	CoverageStatusSuperseded   CoverageStatus = "superseded"
)

func (s CoverageStatus) SatisfiesReview() bool {
	return s == CoverageStatusReviewed || s == CoverageStatusDeepReviewed
}
