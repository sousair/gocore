package redis

import "errors"

var (
	ErrHostNotSet = errors.New("REDIS_HOST is not set")
	ErrPortNotSet = errors.New("REDIS_PORT is not set")
)
