package review

type OccurrenceRevalidation struct {
	ID             Fingerprint
	CurrentID      Fingerprint
	SourceReviewID uint64
	Line           int
	EndLine        int
	Resolved       bool
}
