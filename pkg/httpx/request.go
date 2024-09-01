package httpx

import (
	"github.com/go-playground/validator"
	"github.com/labstack/echo/v4"
)

func NewRequest[T any](e echo.Context) (*T, error) {
	var req T

	if err := e.Bind(&req); err != nil {
		return nil, ErrBadRequest
	}

	validator := validator.New()

	if err := validator.Struct(req); err != nil {
		return nil, ErrInvalidRequest
	}

	return &req, nil
}
