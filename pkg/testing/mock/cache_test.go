//go:build unit

package mock

import (
	"context"
	"testing"
	"time"

	"github.com/sousair/gocore/pkg/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_SetAndGet(t *testing.T) {
	c := NewCache()
	ctx := context.Background()

	err := c.Set(ctx, "key1", "value1")
	require.NoError(t, err)

	val, err := c.Get(ctx, "key1")
	require.NoError(t, err)
	assert.Equal(t, "value1", val)
	assert.Equal(t, 1, c.SetCalls)
	assert.Equal(t, 1, c.GetCalls)
}

func TestCache_GetNotFound(t *testing.T) {
	c := NewCache()
	ctx := context.Background()

	_, err := c.Get(ctx, "missing")
	assert.ErrorIs(t, err, cache.ErrKeyNotFound)
}

func TestCache_Del(t *testing.T) {
	c := NewCache()
	ctx := context.Background()

	c.Set(ctx, "key1", "value1")
	err := c.Del(ctx, "key1")
	require.NoError(t, err)

	_, err = c.Get(ctx, "key1")
	assert.ErrorIs(t, err, cache.ErrKeyNotFound)
	assert.Equal(t, 1, c.DelCalls)
}

func TestCache_TTLExpiry(t *testing.T) {
	c := NewCache()
	ctx := context.Background()

	c.Set(ctx, "key1", "value1", cache.WithTTL(1*time.Millisecond))
	time.Sleep(5 * time.Millisecond)

	_, err := c.Get(ctx, "key1")
	assert.ErrorIs(t, err, cache.ErrKeyNotFound)
}

func TestCache_Lock(t *testing.T) {
	c := NewCache()
	ctx := context.Background()

	unlock, err := c.Lock(ctx, "my-lock")
	require.NoError(t, err)
	assert.Equal(t, 1, c.LockCalls)

	err = unlock(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, c.UnlockCalls)
}

func TestCache_Keys(t *testing.T) {
	c := NewCache()
	ctx := context.Background()

	c.Set(ctx, "a", "1")
	c.Set(ctx, "b", "2")
	c.Set(ctx, "c", "3")

	assert.Len(t, c.Keys(), 3)
}

func TestCache_Reset(t *testing.T) {
	c := NewCache()
	ctx := context.Background()

	c.Set(ctx, "a", "1")
	c.Get(ctx, "a")
	c.Del(ctx, "a")

	c.Reset()

	assert.Empty(t, c.Keys())
	assert.Equal(t, 0, c.SetCalls)
	assert.Equal(t, 0, c.GetCalls)
	assert.Equal(t, 0, c.DelCalls)
}
