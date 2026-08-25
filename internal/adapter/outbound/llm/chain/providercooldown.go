package chain

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

const maximumProviderCooldown = 24 * time.Hour

func (c *Chain) markProviderCooldown(ctx context.Context, failed outbound.Provider, retryAfter time.Duration) time.Time {
	duration := c.cooldownFor
	if retryAfter > duration {
		duration = retryAfter
	}
	if duration > maximumProviderCooldown {
		duration = maximumProviderCooldown
	}
	if duration <= 0 {
		return time.Time{}
	}
	endsAt := c.clock.Now().Add(duration)
	for _, name := range c.failureDomainProviders(failed) {
		effective, err := c.cooldown.Mark(ctx, name, duration)
		if err != nil {
			c.logger.Warn("쿨다운을 기록하지 못했습니다", "provider", name, "source_provider", failed.Name(), "error", err)
			continue
		}
		if effective.After(endsAt) {
			endsAt = effective
		}
	}
	return endsAt
}

func (c *Chain) failureDomainProviders(failed outbound.Provider) []string {
	domain := failed.Profile().FailureDomain
	if domain == "" {
		return []string{failed.Name()}
	}
	providers := make([]string, 0, len(c.providers))
	seen := map[string]struct{}{}
	for _, candidate := range c.providers {
		if candidate.Profile().FailureDomain != domain {
			continue
		}
		if _, exists := seen[candidate.Name()]; exists {
			continue
		}
		seen[candidate.Name()] = struct{}{}
		providers = append(providers, candidate.Name())
	}
	if _, exists := seen[failed.Name()]; !exists {
		providers = append(providers, failed.Name())
	}
	return providers
}
