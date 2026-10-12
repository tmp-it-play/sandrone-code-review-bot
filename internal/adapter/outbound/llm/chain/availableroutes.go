package chain

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
)

func (c *Chain) AvailableRouteCapacities(ctx context.Context, request llm.Request) ([]llm.RouteCapacity, error) {
	capacities := c.RouteCapacitiesFor(request)
	available := make([]llm.RouteCapacity, 0, len(capacities))
	for _, capacity := range capacities {
		_, cooling, err := c.cooldown.EndsAt(ctx, capacity.Provider)
		if err != nil {
			return nil, err
		}
		if !cooling {
			available = append(available, capacity)
		}
	}
	if len(available) == 0 {
		return capacities, nil
	}
	return available, nil
}
