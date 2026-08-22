package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Deduplicator struct {
	client *redis.Client
}

func NewDeduplicator(client *redis.Client) *Deduplicator {
	return &Deduplicator{client: client}
}

func (d *Deduplicator) FirstSeen(ctx context.Context, key string, retention time.Duration) (bool, error) {
	stored, err := d.client.SetNX(ctx, "sandrone:delivery:"+key, "1", retention).Result()
	if err != nil {
		return false, fmt.Errorf("중복 여부를 확인하지 못했다: %w", err)
	}
	return stored, nil
}
