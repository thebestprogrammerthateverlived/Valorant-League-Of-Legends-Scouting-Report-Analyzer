package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisClient wraps the go-redis client
type RedisClient struct {
	Client *redis.Client
}

// NewRedisClient parses REDIS_URL and handles Redis Cloud authentication
func NewRedisClient(redisURL string) (*RedisClient, error) {
	// redis.ParseURL handles the format redis://:password@host:port
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis URL: %v", err)
	}

	client := redis.NewClient(opts)

	// Connection retry logic (3 attempts, 2s delay)
	var lastErr error
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		err := client.Ping(ctx).Err()
		if err == nil {
			fmt.Println("Successfully connected to Redis Cloud")
			return &RedisClient{Client: client}, nil
		}
		lastErr = err
		fmt.Printf("Attempt %d: Failed to connect to Redis, retrying in 2s... (%v)\n", i+1, err)
		time.Sleep(2 * time.Second)
	}

	return nil, fmt.Errorf("failed to connect to Redis after 3 attempts: %v", lastErr)
}

func (r *RedisClient) HealthCheck(ctx context.Context) bool {
	err := r.Client.Ping(ctx).Err()
	return err == nil
}

func (r *RedisClient) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return r.Client.Set(ctx, key, data, ttl).Err()
}

func (r *RedisClient) Get(ctx context.Context, key string, dest interface{}) error {
	data, err := r.Client.Get(ctx, key).Result()
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(data), dest)
}
