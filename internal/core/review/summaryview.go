package review

type SummaryView struct {
	Summary     Summary
	Fallback    []Finding
	InlineCount int
	Attribution Attribution
	Trigger     Trigger
	Incremental bool
	SkippedDup  int
	Unreviewed  []UnreviewedFile
	BatchCount  int
}
