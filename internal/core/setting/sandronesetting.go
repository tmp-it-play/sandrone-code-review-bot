package setting

import "github.com/it-play/sandrone-code-review-bot/internal/core/progresscomment"

type SandroneSetting struct {
	AutoReview           bool
	AutoReviewOnDraft    bool
	AutoReviewOnPush     bool
	ProgressMessageTheme progresscomment.Theme `json:"-"`
	SummaryPlacement     SummaryPlacement
	Providers            []string
	MaxInstructionChars  int
	MaxReviewBatches     int
	InstructionFiles     []string
}
