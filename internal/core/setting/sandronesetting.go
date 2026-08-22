package setting

type SandroneSetting struct {
	AutoReview          bool
	AutoReviewOnPush    bool
	SummaryPlacement    SummaryPlacement
	Providers           []string
	MaxInstructionChars int
	InstructionFiles    []string
}
