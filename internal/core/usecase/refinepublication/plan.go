package refinepublication

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
	"github.com/it-play/sandrone-code-review-bot/internal/core/textrevision"
)

type Plan struct {
	InputHash string
	Document  textrevision.Document
	Request   llm.Request
	Config    setting.RepoConfig
}
