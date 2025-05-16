package redis

import (
	"context"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

type RedisClient struct {
	client *redis.Client
}

func NewRedisClient() *RedisClient {
	addr := os.Getenv("REDIS_URL")
	if addr == "" {
		addr = "localhost:6379"
	}
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: "", // no password by default
		DB:       0,  // use default DB
	})
	return &RedisClient{client: client}
}

func (r *RedisClient) Set(key string, value interface{}, expiration time.Duration) error {
	return r.client.Set(ctx, key, value, expiration).Err()
}

func (r *RedisClient) Get(key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

func (r *RedisClient) Del(key string) error {
	return r.client.Del(ctx, key).Err()
}
