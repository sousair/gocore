package cache

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

type setOptions struct {
	ttl time.Duration
}

type SetOption func(*setOptions)

type Cache interface {
	Set(ctx context.Context, key string, value string, opts ...SetOption) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
}

type cache struct {
	c *redis.Client
}

var _ Cache = (*cache)(nil)

const redisAddrPattern = "%s:%s"

func New() (*cache, error) {
	host, ok := os.LookupEnv("REDIS_HOST")
	if !ok {
		return nil, ErrHostNotSet
	}

	port, ok := os.LookupEnv("REDIS_PORT")
	if !ok {
		return nil, ErrPortNotSet
	}

	c := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf(redisAddrPattern, host, port),
	})

	return &cache{c}, nil
}

func (c *cache) Set(ctx context.Context, key string, value string, opts ...SetOption) error {
	setOpts := &setOptions{}
	for _, opt := range opts {
		opt(setOpts)
	}

	return c.c.Set(ctx, key, value, setOpts.ttl).Err()
}

func (c *cache) Get(ctx context.Context, key string) (string, error) {
	return c.c.Get(ctx, key).Result()
}

func (c *cache) Delete(ctx context.Context, key string) error {
	return c.c.Del(ctx, key).Err()
}
