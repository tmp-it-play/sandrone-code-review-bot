package chain

import "github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"

func (c *Chain) acquireProvider(candidate outbound.Provider) (func(), bool) {
	slots := c.providerSlots[candidate.Name()]
	if slots == nil {
		return func() {}, true
	}
	select {
	case slots <- struct{}{}:
		return func() {
			<-slots
		}, true
	default:
		return nil, false
	}
}
