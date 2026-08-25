package setting

type SandroneSetting struct {
	AutoReview          bool
	AutoReviewOnDraft   bool
	AutoReviewOnPush    bool
	SummaryPlacement    SummaryPlacement
	Providers           []string
	MaxInstructionChars int
	MaxReviewBatches    int
	InstructionFiles    []string
}
