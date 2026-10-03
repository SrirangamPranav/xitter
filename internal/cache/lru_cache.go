package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/monis/twitter-backend-go/internal/config"
	"github.com/redis/go-redis/v9"
)

type QueryCache struct {
	client *redis.Client
	cfg    *config.Config
	hits   atomic.Uint64
	misses atomic.Uint64
}

type CacheStats struct {
	Hits   uint64  `json:"hits"`
	Misses uint64  `json:"misses"`
	Ratio  float64 `json:"hit_ratio"`
}

func NewQueryCache(client *redis.Client, cfg *config.Config) *QueryCache {
	return &QueryCache{
		client: client,
		cfg:    cfg,
	}
}

// GetOrSet implements the Cache-Aside pattern with type safety
// 1. Checks Redis cache instance
// 2. On hit: returns deserialized value
// 3. On miss: invokes loader (database read), caches result in Redis with TTL, returns value
func GetOrSet[T any](c *QueryCache, ctx context.Context, key string, ttl time.Duration, loader func() (T, error)) (T, error) {
	var zero T

	// 1. Attempt Cache Read
	val, err := c.client.Get(ctx, key).Result()
	if err == nil && val != "" {
		c.hits.Add(1)
		var result T
		if err := json.Unmarshal([]byte(val), &result); err == nil {
			return result, nil
		}
		// If corrupted, ignore and fall through to loader
	} else if err != nil && !errors.Is(err, redis.Nil) {
		log.Printf("[LRU Cache] Warning: cache read error for key %s: %v", key, err)
	}

	c.misses.Add(1)

	// 2. Cache Miss: Execute Loader
	result, err := loader()
	if err != nil {
		return zero, err
	}

	// 3. Cache Populate (Asynchronous or synchronous with short timeout)
	go func(val T) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		data, err := json.Marshal(val)
		if err == nil {
			if setErr := c.client.Set(bgCtx, key, data, ttl).Err(); setErr != nil {
				log.Printf("[LRU Cache] Warning: failed to set key %s: %v", key, setErr)
			}
		}
	}(result)

	return result, nil
}

// Invalidate removes one or more keys from the cache
func (c *QueryCache) Invalidate(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}

// InvalidatePattern removes all keys matching a glob pattern (e.g. cache:feed:user_id:*)
func (c *QueryCache) InvalidatePattern(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		keys, nextCursor, err := c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("failed to scan keys for pattern %s: %w", pattern, err)
		}

		if len(keys) > 0 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("failed to delete keys for pattern %s: %w", pattern, err)
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}

// GetStats returns current cache performance metrics
func (c *QueryCache) GetStats() CacheStats {
	h := c.hits.Load()
	m := c.misses.Load()
	total := h + m
	ratio := 0.0
	if total > 0 {
		ratio = float64(h) / float64(total)
	}
	return CacheStats{
		Hits:   h,
		Misses: m,
		Ratio:  ratio,
	}
}

// Ping verifies cache connectivity
func (c *QueryCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}
