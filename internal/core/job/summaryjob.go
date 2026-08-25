package job

import (
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type SummaryJob struct {
	Target              pullrequest.Target
	Trigger             review.Trigger
	Invoker             string
	Instruction         string
	CommentID           int64
	InThread            bool
	RequestIdentity     string
	OperationKey        string
	OrderKey            string
	RequestedAt         time.Time
	Attempt             int
	FinalizationAttempt bool
	FinalAttempt        bool
}
