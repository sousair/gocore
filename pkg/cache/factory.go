package cache

import (
	"context"
	"time"
)

type (
	SetOptions struct {
		TTL time.Duration
	}
	LockOptions struct {
		Retries    int
		RetryDelay time.Duration
	}
)

type SetOption func(*SetOptions)
type LockOption func(*LockOptions)
type UnlockFn func(ctx context.Context) error

type Cache interface {
	Set(ctx context.Context, key string, value string, opts ...SetOption) error
	Get(ctx context.Context, key string) (string, error)
	Del(ctx context.Context, key string) error
	Lock(ctx context.Context, lockKey string, opts ...LockOption) (UnlockFn, error)
}
