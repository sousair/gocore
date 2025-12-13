package cache

import "time"

func WithTTL(ttl time.Duration) SetOption {
	return func(o *SetOptions) {
		o.TTL = ttl
	}
}

func WithLockRetries(retries int) LockOption {
	return func(o *LockOptions) {
		o.Retries = retries
	}
}

func WithLockRetryDelay(delay time.Duration) LockOption {
	return func(o *LockOptions) {
		o.RetryDelay = delay
	}
}
