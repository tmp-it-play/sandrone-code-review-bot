package review

type OpenOccurrence struct {
	ID             Fingerprint
	RootID         Fingerprint
	SourceReviewID uint64
	Path           string
	Line           int
	EndLine        int
	Evidence       string
}
