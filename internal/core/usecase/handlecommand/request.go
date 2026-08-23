package handlecommand

import (
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/command"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type Request struct {
	Target            pullrequest.Target
	Command           command.Command
	PullRequestAuthor string
	RequestIdentity   string
	OccurredAt        time.Time
}
