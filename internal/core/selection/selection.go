package selection

import "github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"

type Selection struct {
	Files   []pullrequest.ChangedFile
	Skipped []pullrequest.ChangedFile
}

func (s Selection) IsEmpty() bool {
	return len(s.Files) == 0
}
