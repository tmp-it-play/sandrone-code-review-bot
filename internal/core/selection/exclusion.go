package selection

import "github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"

type Exclusion struct {
	File   pullrequest.ChangedFile
	Reason ExclusionReason
}
