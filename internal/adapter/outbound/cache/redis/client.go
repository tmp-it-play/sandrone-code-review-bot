package redis

import "github.com/redis/go-redis/v9"

func NewClient(address string, password string, database int) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     address,
		Password: password,
		DB:       database,
	})
}
