package job

import "github.com/it-play/sandrone-code-review-bot/internal/core/progresscomment"

type ReviewAdmission struct {
	Accepted             bool
	ProgressRecoverable  bool
	ProgressMessageTheme progresscomment.Theme
}
