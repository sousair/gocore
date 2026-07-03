package telemetry_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/sousair/gocore/pkg/telemetry"
)

// withOTelGlobals installs tp/prop as the global tracer provider/propagator
// for the duration of the test, restoring whatever was there before on
// cleanup. EchoMiddleware captures both from the globals at construction
// time (mirroring real boot order: telemetry.Init runs before the app
// registers the middleware), so tests must set them up first.
func withOTelGlobals(t *testing.T, tp trace.TracerProvider, prop propagation.TextMapPropagator) {
	t.Helper()
	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(prop)
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})
}

func withCapturedDefaultLogger(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

func TestEchoMiddleware(t *testing.T) {
	t.Run("given no incoming trace header/when request handled/then handler observes valid span and access log emitted", func(t *testing.T) {
		rec := tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
		withOTelGlobals(t, tp, propagation.TraceContext{})
		buf := withCapturedDefaultLogger(t)

		e := echo.New()
		e.Use(telemetry.EchoMiddleware())
		var sawValid bool
		e.GET("/things/:id", func(c echo.Context) error {
			sawValid = trace.SpanContextFromContext(c.Request().Context()).IsValid()
			return c.String(http.StatusOK, "ok")
		})

		srv := httptest.NewServer(e)
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/things/42")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()

		if !sawValid {
			t.Fatal("want valid span context observed in handler")
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("want 200, got %d", resp.StatusCode)
		}
		if len(rec.Ended()) != 1 {
			t.Fatalf("want 1 ended span, got %d", len(rec.Ended()))
		}

		var logRec map[string]any
		if err := json.Unmarshal(buf.Bytes(), &logRec); err != nil {
			t.Fatalf("access log not json: %q", buf.String())
		}
		if logRec["msg"] != "http.request" {
			t.Fatalf("want http.request access log, got %v", logRec)
		}
		if logRec["status"] != float64(http.StatusOK) {
			t.Fatalf("want status 200 in access log, got %v", logRec["status"])
		}
	})

	t.Run("given incoming traceparent header/when request handled/then handler span shares the trace id", func(t *testing.T) {
		rec := tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
		withOTelGlobals(t, tp, propagation.TraceContext{})
		withCapturedDefaultLogger(t)

		e := echo.New()
		e.Use(telemetry.EchoMiddleware())
		var gotTraceID string
		e.GET("/x", func(c echo.Context) error {
			gotTraceID = trace.SpanContextFromContext(c.Request().Context()).TraceID().String()
			return c.NoContent(http.StatusOK)
		})

		srv := httptest.NewServer(e)
		defer srv.Close()

		req, err := http.NewRequest(http.MethodGet, srv.URL+"/x", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()

		if gotTraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Fatalf("want incoming trace id honored, got %q", gotTraceID)
		}
	})

	t.Run("given handler returns a plain error/when request handled/then span marked error and status 500 recorded", func(t *testing.T) {
		rec := tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
		withOTelGlobals(t, tp, propagation.TraceContext{})
		buf := withCapturedDefaultLogger(t)

		e := echo.New()
		e.Use(telemetry.EchoMiddleware())
		e.GET("/boom", func(c echo.Context) error {
			return errors.New("kaboom")
		})
		e.HTTPErrorHandler = func(err error, c echo.Context) {
			_ = c.NoContent(http.StatusInternalServerError)
		}

		srv := httptest.NewServer(e)
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/boom")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("want 500, got %d", resp.StatusCode)
		}
		spans := rec.Ended()
		if len(spans) != 1 {
			t.Fatalf("want 1 ended span, got %d", len(spans))
		}
		if len(spans[0].Events()) == 0 || spans[0].Events()[0].Name != "exception" {
			t.Fatal("want exception event recorded on error")
		}

		var logRec map[string]any
		if err := json.Unmarshal(buf.Bytes(), &logRec); err != nil {
			t.Fatalf("access log not json: %q", buf.String())
		}
		if logRec["status"] != float64(http.StatusInternalServerError) {
			t.Fatalf("want status 500 in access log, got %v", logRec["status"])
		}
	})
}
