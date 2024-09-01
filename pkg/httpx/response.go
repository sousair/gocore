package httpx

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type response struct {
	Message string `json:"message"`
}

func NewBadRequestResponse(e echo.Context, err error) error {
	return e.JSON(http.StatusBadRequest, response{Message: err.Error()})
}

func NewUnauthorizedResponse(e echo.Context, err error) error {
	return e.JSON(http.StatusUnauthorized, response{Message: err.Error()})
}

func NewInternalServerErrorResponse(e echo.Context, err error) error {
	return e.JSON(http.StatusInternalServerError, response{Message: err.Error()})
}

func NewNotFoundResponse(e echo.Context, err error) error {
	return e.JSON(http.StatusNotFound, response{Message: err.Error()})
}

func NewOKResponse(e echo.Context, data any) error {
	return e.JSON(http.StatusOK, data)
}

func NewCreatedResponse(e echo.Context, data any) error {
	return e.JSON(http.StatusCreated, data)
}
