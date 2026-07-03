// Package telemetry is the shared observability substrate: a real OTel
// TracerProvider (export env-gated), W3C trace propagation, and a JSON slog
// default logger whose records carry trace_id/span_id.
//
// Boot:    shutdown, err := telemetry.Init(ctx, telemetry.Config{ServiceName: "bygul"})
// Spans:   tracer := telemetry.TracerFromContext(ctx, scopeName)
//
//	ctx, span := tracer.Start(ctx, "Usecase.Create"); defer span.End()
//
// Errors:  telemetry.RecordError(span, err, err.Error())
//
//	slog.ErrorContext(ctx, "create.fetch_failed", telemetry.Err(err))
//
// LLM:     ctx, ls := telemetry.StartLLMCall(ctx, info); ls.End(result, err)
//
// Metrics and the otelecho/otelpgx/otelriver auto-instrumentation slot in
// later behind the same globals with zero call-site changes.
package telemetry

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	// semconv v1.26.0 (pinned by the task brief) predates the
	// DeploymentEnvironmentName helper. resource.Default() in the resolved
	// sdk v1.44.0 bakes in semconv v1.41.0 internally (see
	// sdk/resource/builtin.go), and resource.Merge rejects mixed schema
	// URLs (resource.ErrSchemaURLConflict), so our own resource attrs must
	// use the matching v1.41.0 package rather than the newest-with-the-
	// helper v1.27.0-v1.40.0. v1.41.0 also dropped the
	// DeploymentEnvironmentName(val) convenience func (present in
	// v1.27.0-v1.40.0) in favor of the bare
	// DeploymentEnvironmentNameKey.String(val) — used below. The wire
	// attribute name is unchanged: "deployment.environment.name".
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"

	"github.com/sousair/gocore/pkg/env"
)

// Config configures Init. ServiceName is required; the rest have sane
// defaults for local development.
type Config struct {
	ServiceName    string    // required
	ServiceVersion string    // default "dev"
	Environment    string    // default env.GetEnvAs("DEPLOYMENT_ENV", "dev")
	Writer         io.Writer // default os.Stdout; tests inject a buffer
}

// Init wires the global TracerProvider, W3C trace propagator, and default
// slog logger. OTLP trace+log export activates iff
// OTEL_EXPORTER_OTLP_ENDPOINT is set; stdout JSON logging is always on.
// Returned shutdown flushes exporters; call it on process exit.
func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if cfg.ServiceName == "" {
		return nil, errors.New("telemetry: ServiceName is required")
	}
	if cfg.ServiceVersion == "" {
		cfg.ServiceVersion = "dev"
	}
	if cfg.Environment == "" {
		cfg.Environment = env.GetEnvAs("DEPLOYMENT_ENV", "dev")
	}
	if cfg.Writer == nil {
		cfg.Writer = os.Stdout
	}
	otlpOn := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != ""

	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
		semconv.DeploymentEnvironmentNameKey.String(cfg.Environment),
	))
	if err != nil {
		return nil, err
	}

	// Construct every fallible piece — trace exporter, tracer provider, log
	// exporter, logger provider, handlers — before touching any global
	// state (otel.SetTracerProvider/SetTextMapPropagator, slog.SetDefault).
	// This keeps the invariant that no global mutation and no started
	// goroutine survives an error return. The tracer provider is the one
	// exception: WithBatcher starts its batch-processor goroutine as soon
	// as the provider is constructed, before the log exporter (which can
	// still fail) is built — so if the log exporter construction fails
	// below, we explicitly shut the tracer provider down before returning.
	traceOpts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if otlpOn {
		exp, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, err
		}
		traceOpts = append(traceOpts, sdktrace.WithBatcher(exp))
	}
	tp := sdktrace.NewTracerProvider(traceOpts...)

	lvl := logLevel()
	stdout := newTraceHandler(slog.NewJSONHandler(cfg.Writer, &slog.HandlerOptions{Level: lvl}))
	handler := slog.Handler(stdout)

	var shutdowns []func(context.Context) error
	shutdowns = append(shutdowns, tp.Shutdown)

	if otlpOn {
		logExp, err := otlploghttp.New(ctx)
		if err != nil {
			_ = tp.Shutdown(ctx)
			return nil, err
		}
		lp := sdklog.NewLoggerProvider(
			sdklog.WithResource(res),
			sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
		)
		shutdowns = append(shutdowns, lp.Shutdown)
		bridge := otelslog.NewHandler(cfg.ServiceName, otelslog.WithLoggerProvider(lp))
		// otelslog/sdk-log has no min-severity option, so level-gate the
		// bridge ourselves — otherwise LOG_LEVEL would be bypassed on the
		// OTLP path and Debug records would always ship to the exporter.
		handler = newFanoutHandler(stdout, newLevelHandler(lvl, newTraceHandler(bridge)))
	}

	// Everything above succeeded — safe to mutate globals now.
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	slog.SetDefault(slog.New(handler).With(
		slog.String("service_name", cfg.ServiceName),
		slog.String("service_version", cfg.ServiceVersion),
		slog.String("deployment_environment", cfg.Environment),
	))

	return func(ctx context.Context) error {
		var first error
		for _, fn := range shutdowns {
			if err := fn(ctx); err != nil && first == nil {
				first = err
			}
		}
		return first
	}, nil
}

func logLevel() slog.Level {
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
