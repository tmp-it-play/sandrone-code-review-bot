package settings

type rawSandrone struct {
	AutoReview          *bool    `yaml:"autoReview"`
	AutoReviewOnPush    *bool    `yaml:"autoReviewOnPush"`
	SummaryPlacement    *string  `yaml:"summaryPlacement"`
	Providers           []string `yaml:"providers"`
	MaxInstructionChars *int     `yaml:"maxInstructionChars"`
	MaxReviewBatches    *int     `yaml:"maxReviewBatches"`
	InstructionFiles    []string `yaml:"instructionFiles"`
}
