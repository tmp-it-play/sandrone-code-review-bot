package job

import "github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"

type ReplyJob struct {
	Target       pullrequest.Target
	Invoker      string
	Instruction  string
	CommentID    int64
	FinalAttempt bool
}
