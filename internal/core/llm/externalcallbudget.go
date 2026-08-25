package llm

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

var ErrExternalCallBudgetExhausted = errors.New("공유 외부 호출 예산을 모두 사용했습니다")

var ErrExternalCallBudgetUnavailable = errors.New("공유 외부 호출 예산을 예약하지 못했습니다")

type ExternalCallBudget struct {
	limit     int64
	takeLimit int64
	used      atomic.Int64
	reserve   func() error
	mutex     sync.Mutex
}

func NewExternalCallBudget(limit int) *ExternalCallBudget {
	if limit < 1 {
		limit = 1
	}
	return &ExternalCallBudget{limit: int64(limit), takeLimit: int64(limit)}
}

func NewScopedDurableExternalCallBudget(limit int, used int, takeLimit int, reserve func() error) *ExternalCallBudget {
	budget := NewExternalCallBudget(limit)
	if used < 0 {
		used = 0
	}
	if used > budget.Limit() {
		used = budget.Limit()
	}
	if takeLimit < used {
		takeLimit = used
	}
	if takeLimit > budget.Limit() {
		takeLimit = budget.Limit()
	}
	budget.takeLimit = int64(takeLimit)
	budget.used.Store(int64(used))
	budget.reserve = reserve
	return budget
}

func (b *ExternalCallBudget) Take() bool {
	return b.Reserve() == nil
}

func (b *ExternalCallBudget) Reserve() error {
	if b == nil {
		return ErrExternalCallBudgetExhausted
	}
	if b.reserve != nil {
		b.mutex.Lock()
		defer b.mutex.Unlock()
		used := b.used.Load()
		if used >= b.takeLimit {
			return ErrExternalCallBudgetExhausted
		}
		if err := b.reserve(); err != nil {
			if errors.Is(err, ErrExternalCallBudgetExhausted) {
				b.used.Store(b.takeLimit)
				return ErrExternalCallBudgetExhausted
			}
			return fmt.Errorf("%w: %w", ErrExternalCallBudgetUnavailable, err)
		}
		b.used.Store(used + 1)
		return nil
	}
	for {
		used := b.used.Load()
		if used >= b.takeLimit {
			return ErrExternalCallBudgetExhausted
		}
		if b.used.CompareAndSwap(used, used+1) {
			return nil
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
	if b == nil {
		return 0
	}
	remaining := int(b.takeLimit) - b.Used()
	if remaining < 0 {
		return 0
	}
	return remaining
}
