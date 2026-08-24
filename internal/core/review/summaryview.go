package review

type SummaryView struct {
	Summary         Summary
	Fallback        []Finding
	InlineCount     int
	Attribution     Attribution
	Style           Style
	Trigger         Trigger
	Incremental     bool
	OmittedFindings int
}
