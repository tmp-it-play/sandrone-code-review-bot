package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cooldown struct {
	client *redis.Client
}

func NewCooldown(client *redis.Client) *Cooldown {
	return &Cooldown{client: client}
}

func (c *Cooldown) Active(ctx context.Context, provider string) (bool, error) {
	count, err := c.client.Exists(ctx, cooldownKey(provider)).Result()
	if err != nil {
		return false, fmt.Errorf("쿨다운 상태를 읽지 못했습니다: %w", err)
	}
	return count > 0, nil
}

func (c *Cooldown) Mark(ctx context.Context, provider string, duration time.Duration) error {
	deadline := time.Now().Add(duration).UTC().Format(time.RFC3339)
	if err := c.client.Set(ctx, cooldownKey(provider), deadline, duration).Err(); err != nil {
		return fmt.Errorf("쿨다운을 기록하지 못했습니다: %w", err)
	}
	return nil
}

func (c *Cooldown) EndsAt(ctx context.Context, provider string) (time.Time, bool, error) {
	value, err := c.client.Get(ctx, cooldownKey(provider)).Result()
	if err == redis.Nil {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("쿨다운 만료 시각을 읽지 못했습니다: %w", err)
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false, nil
	}
	return parsed, true, nil
}

func cooldownKey(provider string) string {
	return "sandrone:cooldown:" + provider
}
