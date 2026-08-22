package instruction

type Budget struct {
	total     int
	remaining int
}

func NewBudget(total int) *Budget {
	if total < 0 {
		total = 0
	}
	return &Budget{total: total, remaining: total}
}

func (b *Budget) Remaining() int {
	return b.remaining
}

func (b *Budget) Exhausted() bool {
	return b.remaining <= 0
}

func (b *Budget) PerDocumentLimit() int {
	return b.total / 2
}

func (b *Budget) Take(content string) (string, bool) {
	if b.Exhausted() {
		return "", false
	}
	allowance := b.remaining
	if limit := b.PerDocumentLimit(); limit > 0 && limit < allowance {
		allowance = limit
	}
	if len(content) <= allowance {
		b.remaining -= len(content)
		return content, false
	}
	b.remaining -= allowance
	return content[:allowance], true
}
