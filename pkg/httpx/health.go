package httpx

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type HealthCheck struct {
	Key  string
	Func func(c echo.Context) error
}

type HealthStatus string

const (
	HealthStatusOK  HealthStatus = "ok"
	HealthStatusErr HealthStatus = "error"
)

type CheckStatus struct {
	Status  HealthStatus `json:"status"`
	Message string       `json:"message,omitempty"`
}

func WithHealthRoute(server *echo.Echo, checks ...*HealthCheck) {
	server.GET("/health", func(c echo.Context) error {
		checkMap := make(map[string]CheckStatus)
		overallStatus := HealthStatusOK

		for _, check := range checks {
			status := CheckStatus{Status: HealthStatusOK}

			if err := check.Func(c); err != nil {
				overallStatus = HealthStatusErr

				status.Status = HealthStatusErr
				status.Message = err.Error()
			}

			checkMap[check.Key] = status
		}

		res := map[string]any{
			"status": overallStatus,
			"checks": checkMap,
		}

		if overallStatus != HealthStatusOK {
			return c.JSON(http.StatusServiceUnavailable, res)
		}

		return NewOKResponse(c, res)
	})
}
