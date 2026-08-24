package health

import (
	"context"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Checker struct {
	database *gorm.DB
	cache    *redis.Client
}

func NewChecker(database *gorm.DB, cache *redis.Client) *Checker {
	return &Checker{database: database, cache: cache}
}

func (c *Checker) Check(ctx context.Context) (map[string]string, bool) {
	status := map[string]string{"mysql": "ok", "redis": "ok"}
	ready := true
	pool, err := c.database.DB()
	if err != nil || pool.PingContext(ctx) != nil {
		status["mysql"] = "unreachable"
		ready = false
	}
	if err := c.cache.Ping(ctx).Err(); err != nil {
		status["redis"] = "degraded"
	}
	return status, ready
}
