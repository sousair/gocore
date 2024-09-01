package database

import (
	"errors"
)

var (
	ErrDBUsernameNotSet = errors.New("DB_USERNAME is not set")
	ErrDBPasswordNotSet = errors.New("DB_PASSWORD is not set")
	ErrDBHostNotSet     = errors.New("DB_HOST is not set")
	ErrDBNameNotSet     = errors.New("DB_NAME is not set")

	ErrBadEntity = errors.New("the provided value does not implement the Entity interface")
	ErrNotFound  = errors.New("record not found")
)
