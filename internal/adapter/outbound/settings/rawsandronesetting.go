package settings

type rawSandroneSetting struct {
	AutoReview           *bool     `yaml:"autoReview"`
	AutoReviewOnDraft    *bool     `yaml:"autoReviewOnDraft"`
	AutoReviewOnPush     *bool     `yaml:"autoReviewOnPush"`
	ProgressMessageTheme *string   `yaml:"progressMessageTheme"`
	SummaryPlacement     *string   `yaml:"summaryPlacement"`
	Providers            *[]string `yaml:"providers"`
	MaxInstructionChars  *int      `yaml:"maxInstructionChars"`
	MaxReviewBatches     *int      `yaml:"maxReviewBatches"`
	InstructionFiles     *[]string `yaml:"instructionFiles"`
}
