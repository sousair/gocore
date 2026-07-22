package telemetry

import (
	"fmt"
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

// EchoOption configures EchoMiddleware.
type EchoOption func(*echoConfig)

type echoConfig struct{ skipRoutes map[string]struct{} }

// WithSkipRoutes replaces the default probe-route skip set (default {"/health"}).
// Routes are matched against echo's route template, c.Path(). A skipped route
// gets no span and no RED metric, and logs a single WARN line only when it
// fails (status >= 400 or a handler error) — so a DB-down readiness probe stays
// visible while healthy probes are silent.
func WithSkipRoutes(routes ...string) EchoOption {
	return func(cfg *echoConfig) {
		cfg.skipRoutes = make(map[string]struct{}, len(routes))
		for _, r := range routes {
			cfg.skipRoutes[r] = struct{}{}
		}
	}
}

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
func EchoMiddleware(opts ...EchoOption) echo.MiddlewareFunc {
	cfg := echoConfig{skipRoutes: map[string]struct{}{"/health": {}}}
	for _, opt := range opts {
		opt(&cfg)
	}

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
			if _, skip := cfg.skipRoutes[c.Path()]; skip {
				return serveProbe(c, next)
			}

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

			status := statusOf(c, err)
			if err != nil {
				RecordError(span, err, err.Error())
			}
			span.SetAttributes(attribute.Int("http.status_code", status))
			if status >= 500 && err == nil {
				span.SetStatus(codes.Error, http.StatusText(status))
			}

			ms := time.Since(start).Milliseconds()
			slog.InfoContext(ctx, fmt.Sprintf("%s %s %d %dms", req.Method, c.Path(), status, ms),
				"event", "http.request",
				"method", req.Method, "route", c.Path(), "status", status,
				"duration_ms", ms)

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

// statusOf resolves the response status, mapping a handler error to the code
// echo's error handler will send (an *echo.HTTPError's own code, else 500).
func statusOf(c echo.Context, err error) int {
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code
		}
		return http.StatusInternalServerError
	}
	return c.Response().Status
}

// serveProbe runs a skipped-route handler with no span and no RED metric,
// emitting a single WARN access-log only when the probe itself fails so a
// DB-down readiness probe stays visible while healthy probes go silent.
func serveProbe(c echo.Context, next echo.HandlerFunc) error {
	err := next(c)
	status := statusOf(c, err)
	if status >= 400 || err != nil {
		slog.WarnContext(c.Request().Context(),
			fmt.Sprintf("probe %s %d", c.Path(), status),
			"event", "http.probe", "route", c.Path(), "status", status)
	}
	return err
}
