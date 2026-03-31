//go:build unit

package mock

import (
	"context"
	"sync"
	"time"

	"github.com/sousair/gocore/pkg/cache"
)

type Cache struct {
	mu   sync.RWMutex
	data map[string]cacheEntry

	SetCalls    int
	GetCalls    int
	DelCalls    int
	LockCalls   int
	UnlockCalls int
}

type cacheEntry struct {
	value     string
	expiresAt *time.Time
}

var _ cache.Cache = (*Cache)(nil)

func NewCache() *Cache {
	return &Cache{
		data: make(map[string]cacheEntry),
	}
}

func (c *Cache) Set(ctx context.Context, key string, value string, opts ...cache.SetOption) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	options := &cache.SetOptions{}
	for _, opt := range opts {
		opt(options)
	}

	entry := cacheEntry{value: value}
	if options.TTL > 0 {
		exp := time.Now().Add(options.TTL)
		entry.expiresAt = &exp
	}

	c.data[key] = entry
	c.SetCalls++
	return nil
}

func (c *Cache) Get(ctx context.Context, key string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	c.GetCalls++

	entry, ok := c.data[key]
	if !ok {
		return "", cache.ErrKeyNotFound
	}

	if entry.expiresAt != nil && time.Now().After(*entry.expiresAt) {
		return "", cache.ErrKeyNotFound
	}

	return entry.value, nil
}

func (c *Cache) Del(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.data, key)
	c.DelCalls++
	return nil
}

func (c *Cache) Lock(ctx context.Context, lockKey string, opts ...cache.LockOption) (cache.UnlockFn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.LockCalls++

	return func(ctx context.Context) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.UnlockCalls++
		return nil
	}, nil
}

func (c *Cache) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var keys []string
	for k, entry := range c.data {
		if entry.expiresAt != nil && time.Now().After(*entry.expiresAt) {
			continue
		}
		keys = append(keys, k)
	}
	return keys
}

func (c *Cache) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = make(map[string]cacheEntry)
	c.SetCalls = 0
	c.GetCalls = 0
	c.DelCalls = 0
	c.LockCalls = 0
	c.UnlockCalls = 0
}
