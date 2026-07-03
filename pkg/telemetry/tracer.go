package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// TracerFromContext returns a tracer from the provider that created the
// ambient span, so child spans stay in the caller's trace even under a
// non-global provider (tests). Falls back to the global provider.
// Convention: scopeName is a per-package constant, e.g.
// "github.com/sousair/bygul/internal/modules/transaction/usecase".
func TracerFromContext(ctx context.Context, scopeName string) trace.Tracer {
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		return span.TracerProvider().Tracer(scopeName)
	}
	return otel.Tracer(scopeName)
}

// RecordError marks the span failed: records the exception event and sets
// status Error. Pair it with a slog.ErrorContext call (double-record pattern).
func RecordError(span trace.Span, err error, msg string) {
	if span == nil || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, msg)
}
