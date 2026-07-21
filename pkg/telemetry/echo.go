package telemetry

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const echoScope = "github.com/sousair/gocore/pkg/telemetry/echo"

// EchoMiddleware is the app entry-point middleware: it extracts an incoming
// W3C trace context (if any), starts the request's root span, emits the
// "http.request" access-log record, and records the
// http.server.request.duration histogram (RED: rate/errors/duration in one
// instrument) — apps register this instead of their own
// RequestLoggerWithConfig.
//
// The tracer, propagator, and histogram are captured once, from the globals
// telemetry.Init sets up, at the moment this constructor runs — register it
// after Init, same as any other otel-backed middleware.
func EchoMiddleware() echo.MiddlewareFunc {
	tracer := otel.Tracer(echoScope)
	prop := otel.GetTextMapPropagator()
	// Float64Histogram only errors on a malformed instrument config (a bug), so panic at wire time.
	hist, err := otel.Meter(echoScope).Float64Histogram(
		"http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of HTTP server requests."),
	)
	if err != nil {
		panic(err)
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			ctx := prop.Extract(req.Context(), propagation.HeaderCarrier(req.Header))
			ctx, span := tracer.Start(ctx, "HTTP "+req.Method+" "+c.Path(),
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("http.method", req.Method),
					attribute.String("http.route", c.Path()),
				))
			defer span.End()
			c.SetRequest(req.WithContext(ctx))

			start := time.Now()
			err := next(c)

			status := c.Response().Status
			if err != nil {
				if he, ok := err.(*echo.HTTPError); ok {
					status = he.Code
				} else {
					status = http.StatusInternalServerError
				}
				RecordError(span, err, err.Error())
			}
			span.SetAttributes(attribute.Int("http.status_code", status))
			if status >= 500 && err == nil {
				span.SetStatus(codes.Error, http.StatusText(status))
			}

			slog.InfoContext(ctx, "http.request",
				"method", req.Method, "route", c.Path(), "status", status,
				"duration_ms", time.Since(start).Milliseconds())

			// c.Path() (the route template) keeps attribute cardinality
			// bounded — the raw URL would blow it up with path params.
			hist.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(
				attribute.String("http.request.method", req.Method),
				attribute.String("http.route", c.Path()),
				attribute.Int("http.response.status_code", status),
			))

			return err
		}
	}
}
