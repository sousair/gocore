package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestWithHealthRoute_AllChecksPass_Returns200(t *testing.T) {
	server := echo.New()
	WithHealthRoute(server, &HealthCheck{
		Key:  "db",
		Func: func(c echo.Context) error { return nil },
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body.Status != string(HealthStatusOK) {
		t.Fatalf("expected status %q, got %q", HealthStatusOK, body.Status)
	}
}

func TestWithHealthRoute_CheckFails_Returns503(t *testing.T) {
	server := echo.New()
	WithHealthRoute(server, &HealthCheck{
		Key:  "db",
		Func: func(c echo.Context) error { return errors.New("connection refused") },
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body.Status != string(HealthStatusErr) {
		t.Fatalf("expected status %q, got %q", HealthStatusErr, body.Status)
	}
}
