package llm

import (
	"sync"
	"sync/atomic"
)

type ExternalCallBudget struct {
	limit   int64
	used    atomic.Int64
	reserve func() bool
	mutex   sync.Mutex
}

func NewExternalCallBudget(limit int) *ExternalCallBudget {
	if limit < 1 {
		limit = 1
	}
	return &ExternalCallBudget{limit: int64(limit)}
}

func NewDurableExternalCallBudget(limit int, reserve func() bool) *ExternalCallBudget {
	budget := NewExternalCallBudget(limit)
	budget.reserve = reserve
	return budget
}

func (b *ExternalCallBudget) Take() bool {
	if b == nil {
		return false
	}
	if b.reserve != nil {
		b.mutex.Lock()
		defer b.mutex.Unlock()
		used := b.used.Load()
		if used >= b.limit {
			return false
		}
		if !b.reserve() {
			return false
		}
		b.used.Store(used + 1)
		return true
	}
	for {
		used := b.used.Load()
		if used >= b.limit {
			return false
		}
		if b.used.CompareAndSwap(used, used+1) {
			return true
		}
	}
}

func (b *ExternalCallBudget) Limit() int {
	if b == nil {
		return 0
	}
	return int(b.limit)
}

func (b *ExternalCallBudget) Used() int {
	if b == nil {
		return 0
	}
	return int(b.used.Load())
}

func (b *ExternalCallBudget) Remaining() int {
	remaining := b.Limit() - b.Used()
	if remaining < 0 {
		return 0
	}
	return remaining
}
