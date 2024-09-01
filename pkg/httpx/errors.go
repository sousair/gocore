package httpx

import "errors"

var (
	ErrBadRequest     = errors.New("bad request")
	ErrInvalidRequest = errors.New("invalid request")
)
