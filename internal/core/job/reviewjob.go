package job

import (
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type ReviewJob struct {
	Target             pullrequest.Target
	Trigger            review.Trigger
	Invoker            string
	Instruction        string
	CommentID          int64
	InThread           bool
	RequestIdentity    string
	RequestReceivedAt  time.Time
	SnapshotObservedAt time.Time
	SnapshotOrderKey   string
	Attempt            int
	FinalAttempt       bool
	Incremental        bool
}
