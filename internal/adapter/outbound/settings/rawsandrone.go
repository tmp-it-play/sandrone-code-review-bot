package settings

type rawSandrone struct {
	Review              rawReviewSetting `yaml:",inline"`
	AutoReview          *bool            `yaml:"autoReview"`
	AutoReviewOnDraft   *bool            `yaml:"autoReviewOnDraft"`
	AutoReviewOnPush    *bool            `yaml:"autoReviewOnPush"`
	SummaryPlacement    *string          `yaml:"summaryPlacement"`
	Providers           []string         `yaml:"providers"`
	MaxInstructionChars *int             `yaml:"maxInstructionChars"`
	MaxReviewBatches    *int             `yaml:"maxReviewBatches"`
	InstructionFiles    []string         `yaml:"instructionFiles"`
}
