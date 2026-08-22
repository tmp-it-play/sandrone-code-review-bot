package review

type SummaryView struct {
	Summary     Summary
	Fallback    []Finding
	InlineCount int
	Provider    string
	Model       string
	Trigger     Trigger
	Incremental bool
	SkippedDup  int
}
