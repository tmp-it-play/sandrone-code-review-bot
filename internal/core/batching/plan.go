package batching

import "github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"

type Plan struct {
	Batches  [][]pullrequest.ChangedFile
	Overflow []pullrequest.ChangedFile
}

func (p Plan) Count() int {
	return len(p.Batches)
}

func (p Plan) IsSplit() bool {
	return len(p.Batches) > 1
}
