package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache is a general-purpose Redis-backed key-value cache.
// All methods gracefully handle a nil Redis client (become no-ops).
type Cache struct {
	rdb    *redis.Client
	ttl    time.Duration
	prefix string
}

// New creates a new Redis-backed cache.
// prefix is prepended to all keys (e.g. "minicc:cache:").
// ttl is the default TTL for cached entries.
func New(rdb *redis.Client, prefix string, ttl time.Duration) *Cache {
	return &Cache{
		rdb:    rdb,
		ttl:    ttl,
		prefix: prefix,
	}
}

// key returns the full Redis key with prefix.
func (c *Cache) key(k string) string {
	return c.prefix + k
}

// Get retrieves a cached entry. Returns (nil, false) on miss or if Redis is nil.
func (c *Cache) Get(ctx context.Context, key string) ([]byte, bool) {
	if c.rdb == nil {
		return nil, false
	}
	data, err := c.rdb.Get(ctx, c.key(key)).Bytes()
	if err != nil {
		return nil, false
	}
	return data, true
}

// Set stores a value with the default TTL.
func (c *Cache) Set(ctx context.Context, key string, value []byte) {
	if c.rdb == nil {
		return
	}
	c.rdb.Set(ctx, c.key(key), value, c.ttl)
}

// SetTTL stores a value with a custom TTL.
func (c *Cache) SetTTL(ctx context.Context, key string, value []byte, ttl time.Duration) {
	if c.rdb == nil {
		return
	}
	c.rdb.Set(ctx, c.key(key), value, ttl)
}

// Delete removes a cached entry.
func (c *Cache) Delete(ctx context.Context, key string) {
	if c.rdb == nil {
		return
	}
	c.rdb.Del(ctx, c.key(key))
}

// Exists checks if a key exists in the cache.
func (c *Cache) Exists(ctx context.Context, key string) bool {
	if c.rdb == nil {
		return false
	}
	n, err := c.rdb.Exists(ctx, c.key(key)).Result()
	return err == nil && n > 0
}

// GetOrSet attempts to get a cached value; if missing, calls fn, stores the result,
// and returns it. fn is only called if the cache is a miss or Redis is nil.
func (c *Cache) GetOrSet(ctx context.Context, key string, fn func() ([]byte, error)) ([]byte, error) {
	if data, ok := c.Get(ctx, key); ok {
		return data, nil
	}
	data, err := fn()
	if err != nil {
		return nil, fmt.Errorf("get or set: %w", err)
	}
	if data != nil {
		c.Set(ctx, key, data)
	}
	return data, nil
}

// GetJSON is a typed variant: deserializes the cached value into dst.
func (c *Cache) GetJSON(ctx context.Context, key string, dst interface{}) (bool, error) {
	data, ok := c.Get(ctx, key)
	if !ok {
		return false, nil
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return false, fmt.Errorf("cache get json: %w", err)
	}
	return true, nil
}

// SetJSON serializes and stores a value.
func (c *Cache) SetJSON(ctx context.Context, key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache set json: %w", err)
	}
	c.Set(ctx, key, data)
	return nil
}

// Stats returns basic cache statistics (only key count for now).
func (c *Cache) Stats(ctx context.Context) map[string]int64 {
	if c.rdb == nil {
		return map[string]int64{"keys": 0}
	}
	// Approximate key count by scanning
	count := int64(0)
	iter := c.rdb.Scan(ctx, 0, c.prefix+"*", 1000).Iterator()
	for iter.Next(ctx) {
		count++
	}
	return map[string]int64{"keys": count}
}
