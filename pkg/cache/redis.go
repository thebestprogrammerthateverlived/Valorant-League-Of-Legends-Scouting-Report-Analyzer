// pkg/cache/redis.go - FINAL VERSION
package cache

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "time"

    "github.com/redis/go-redis/v9"
)

type RedisClient struct {
    client *redis.Client
}

// NewRedisClient creates a new Redis client with connection retry logic
func NewRedisClient(url string) (*RedisClient, error) {
    opt, err := redis.ParseURL(url)
    if err != nil {
        return nil, fmt.Errorf("invalid Redis URL: %w", err)
    }

    // Configure connection pool
    opt.PoolSize = 10
    opt.MinIdleConns = 5
    opt.MaxRetries = 3
    opt.DialTimeout = 5 * time.Second
    opt.ReadTimeout = 3 * time.Second
    opt.WriteTimeout = 3 * time.Second

    client := redis.NewClient(opt)

    // Test connection with retries
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    for i := 0; i < 3; i++ {
        if err := client.Ping(ctx).Err(); err == nil {
            log.Printf("✅ Successfully connected to Redis")
            return &RedisClient{client: client}, nil
        }
        log.Printf("⚠️ Redis connection attempt %d failed, retrying...", i+1)
        time.Sleep(time.Second * 2)
    }

    return nil, fmt.Errorf("failed to connect to Redis after 3 attempts")
}

// Get retrieves and unmarshals a JSON value from cache
func (r *RedisClient) Get(ctx context.Context, key string, dest interface{}) error {
    val, err := r.client.Get(ctx, key).Result()
    
    if err == redis.Nil {
        log.Printf("📭 Cache miss for key: %s", key)
        return fmt.Errorf("cache miss")
    }
    
    if err != nil {
        log.Printf("❌ Redis error for key '%s': %v", key, err)
        return fmt.Errorf("redis error: %w", err)
    }
    
    if err := json.Unmarshal([]byte(val), dest); err != nil {
        log.Printf("❌ Failed to unmarshal cached value for key '%s': %v", key, err)
        return fmt.Errorf("failed to unmarshal: %w", err)
    }
    
    log.Printf("✅ Cache hit for key: %s", key)
    return nil
}

// Set marshals and stores a value as JSON in cache
func (r *RedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
    jsonBytes, err := json.Marshal(value)
    if err != nil {
        return fmt.Errorf("failed to marshal value: %w", err)
    }

    err = r.client.Set(ctx, key, jsonBytes, expiration).Err()
    if err != nil {
        log.Printf("❌ Failed to set cache key '%s': %v", key, err)
        return fmt.Errorf("failed to set cache: %w", err)
    }

    log.Printf("✅ Cached key '%s' with TTL %v", key, expiration)
    return nil
}

// Delete removes a key from cache
func (r *RedisClient) Delete(ctx context.Context, key string) error {
    err := r.client.Del(ctx, key).Err()
    if err != nil {
        log.Printf("❌ Failed to delete cache key '%s': %v", key, err)
        return err
    }
    log.Printf("🗑️ Deleted cache key: %s", key)
    return nil
}

// Exists checks if a key exists in cache
func (r *RedisClient) Exists(ctx context.Context, key string) (bool, error) {
    result, err := r.client.Exists(ctx, key).Result()
    if err != nil {
        return false, err
    }
    return result > 0, nil
}

// Close closes the Redis connection
func (r *RedisClient) Close() error {
    return r.client.Close()
}

// Ping checks if Redis is responsive
func (r *RedisClient) Ping(ctx context.Context) error {
    return r.client.Ping(ctx).Err()
}

// HealthCheck returns true if Redis is healthy
func (r *RedisClient) HealthCheck(ctx context.Context) bool {
    err := r.client.Ping(ctx).Err()
    return err == nil
}

// GetString retrieves a raw string value
func (r *RedisClient) GetString(ctx context.Context, key string) (string, error) {
    val, err := r.client.Get(ctx, key).Result()
    
    if err == redis.Nil {
        log.Printf("📭 Cache miss for key: %s", key)
        return "", fmt.Errorf("cache miss")
    }
    
    if err != nil {
        log.Printf("❌ Redis error for key '%s': %v", key, err)
        return "", fmt.Errorf("redis error: %w", err)
    }
    
    log.Printf("✅ Cache hit for key: %s", key)
    return val, nil
}

// SetString stores a raw string value
func (r *RedisClient) SetString(ctx context.Context, key string, value string, expiration time.Duration) error {
    err := r.client.Set(ctx, key, value, expiration).Err()
    if err != nil {
        log.Printf("❌ Failed to set cache key '%s': %v", key, err)
        return fmt.Errorf("failed to set cache: %w", err)
    }
    log.Printf("✅ Cached key '%s' with TTL %v", key, expiration)
    return nil
}