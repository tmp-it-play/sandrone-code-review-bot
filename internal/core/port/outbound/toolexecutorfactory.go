package outbound

import "github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"

type ToolExecutorFactory interface {
	ForTarget(target pullrequest.Target, ref string, maxReads int) ToolExecutor
}
