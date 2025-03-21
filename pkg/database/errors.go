package database

import (
	"errors"
)

var (
	ErrUsernameNotSet = errors.New("DB_USERNAME is not set")
	ErrPasswordNotSet = errors.New("DB_PASSWORD is not set")
	ErrHostNotSet     = errors.New("DB_HOST is not set")
	ErrNameNotSet     = errors.New("DB_NAME is not set")

	ErrBadEntity = errors.New("the provided value does not implement the Entity interface")
	ErrNotFound  = errors.New("record not found")
)
