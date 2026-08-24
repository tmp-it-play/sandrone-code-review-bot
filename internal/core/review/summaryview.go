package review

type SummaryView struct {
	Summary                 Summary
	Fallback                []Finding
	InlineCount             int
	Attribution             Attribution
	Style                   Style
	Trigger                 Trigger
	Incremental             bool
	SkippedDup              int
	OmittedFindings         int
	VerificationUnavailable bool
	Unreviewed              []UnreviewedFile
	Coverage                CoverageView
}
