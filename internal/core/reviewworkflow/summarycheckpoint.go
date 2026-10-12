package reviewworkflow

import "github.com/it-play/sandrone-code-review-bot/internal/core/llm"

type SummaryCheckpoint struct {
	InputHash string
	Overview  string
	Fallback  bool
	Response  llm.Response
}
