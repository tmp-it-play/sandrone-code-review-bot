package selection

import "github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"

type Selection struct {
	Files    []pullrequest.ChangedFile
	Skipped  []pullrequest.ChangedFile
	Excluded []Exclusion
}

func (s Selection) IsEmpty() bool {
	return len(s.Files) == 0
}

func (s Selection) HasUnresolved() bool {
	for _, excluded := range s.Excluded {
		if excluded.Reason == ExclusionReasonPatchUnavailable {
			return true
		}
	}
	return false
}
