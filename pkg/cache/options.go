package cache

import "time"

func WithTTL(ttl time.Duration) SetOption {
	return func(o *setOptions) {
		o.ttl = ttl
	}
}
