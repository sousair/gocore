package redis

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
	"github.com/redis/go-redis/v9"
	"github.com/sousair/gocore/pkg/cache"
)

type Redis struct {
	c  *redis.Client
	rs *redsync.Redsync
}

var _ cache.Cache = (*Redis)(nil)

const redisAddrPattern = "%s:%s"

const (
	DEFAULT_MIN_IDLE_CONNECTIONS = 1
	DEFAULT_MAX_CONNECTIONS      = 5
	DEFAULT_POOL_TIMEOUT_SEC     = 5
)

func New() (*Redis, error) {
	host, ok := os.LookupEnv("REDIS_HOST")
	if !ok {
		return nil, ErrHostNotSet
	}

	port, ok := os.LookupEnv("REDIS_PORT")
	if !ok {
		return nil, ErrPortNotSet
	}

	clientOpts := &redis.Options{
		Addr: fmt.Sprintf(redisAddrPattern, host, port),
	}

	if minIdleConn, ok := os.LookupEnv("REDIS_MIN_IDLE_CONNECTIONS"); ok {
		conv, err := strconv.Atoi(minIdleConn)
		if err != nil {
			return nil, fmt.Errorf("invalid REDIS_MIN_IDLE_CONNECTIONS value: %v", err)
		}
		clientOpts.MinIdleConns = conv
	} else {
		slog.Warn("REDIS_MIN_IDLE_CONNECTIONS is not set, using default",
			slog.Int("DEFAULT_MIN_IDLE_CONNECTIONS", DEFAULT_MIN_IDLE_CONNECTIONS),
		)
		clientOpts.MinIdleConns = DEFAULT_MIN_IDLE_CONNECTIONS
	}

	if maxConn, ok := os.LookupEnv("REDIS_MAX_CONNECTIONS"); ok {
		conv, err := strconv.Atoi(maxConn)
		if err != nil {
			return nil, fmt.Errorf("invalid REDIS_MAX_CONNECTIONS value: %v", err)
		}
		clientOpts.PoolSize = conv
	} else {
		slog.Warn("REDIS_MAX_CONNECTIONS is not set, using default",
			slog.Int("DEFAULT_MAX_CONNECTIONS", DEFAULT_MAX_CONNECTIONS),
		)
		clientOpts.PoolSize = DEFAULT_MAX_CONNECTIONS
	}

	if timeout, ok := os.LookupEnv("REDIS_POOL_TIMEOUT_SEC"); ok {
		conv, err := strconv.Atoi(timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid REDIS_POOL_TIMEOUT_SEC value: %v", err)
		}

		clientOpts.PoolTimeout = time.Duration(conv) * time.Second
	} else {
		slog.Warn("REDIS_POOL_TIMEOUT_SEC is not set, using default",
			slog.Int("DEFAULT_POOL_TIMEOUT_SEC", DEFAULT_POOL_TIMEOUT_SEC),
		)
		clientOpts.PoolTimeout = DEFAULT_POOL_TIMEOUT_SEC * time.Second
	}

	c := redis.NewClient(clientOpts)

	return &Redis{
		c:  c,
		rs: redsync.New(goredis.NewPool(c)),
	}, nil
}

func (c *Redis) GetClient() *redis.Client {
	return c.c
}

func (c *Redis) Set(ctx context.Context, key string, value string, opts ...cache.SetOption) error {
	setOpts := &cache.SetOptions{}
	for _, opt := range opts {
		opt(setOpts)
	}

	return c.c.Set(ctx, key, value, setOpts.TTL).Err()
}

func (c *Redis) Get(ctx context.Context, key string) (string, error) {
	val, err := c.c.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", cache.ErrKeyNotFound
		}

		return "", err
	}

	return val, nil
}

func (c *Redis) Del(ctx context.Context, key string) error {
	return c.c.Del(ctx, key).Err()
}

func (c *Redis) Lock(ctx context.Context, lockKey string, opts ...cache.LockOption) (cache.UnlockFn, error) {
	opt := new(cache.LockOptions)
	for _, o := range opts {
		o(opt)
	}

	fmtKey := fmt.Sprintf("rs_lock:%s", lockKey)
	var mutexOptions []redsync.Option
	if opt.Retries > 0 {
		mutexOptions = append(mutexOptions, redsync.WithTries(opt.Retries))
	}

	if opt.RetryDelay > 0 {
		mutexOptions = append(mutexOptions, redsync.WithRetryDelay(opt.RetryDelay))
	}

	mutex := c.rs.NewMutex(fmtKey, mutexOptions...)
	if err := mutex.LockContext(ctx); err != nil {
		return nil, fmt.Errorf("[Redis] failed to acquire lock: %v", err)
	}

	return func(ctx context.Context) error {
		if _, err := mutex.UnlockContext(ctx); err != nil {
			slog.ErrorContext(ctx, "[Redis] failed to release lock", "error", err)
			return err
		}

		return nil
	}, nil
}
