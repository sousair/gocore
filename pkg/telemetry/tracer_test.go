package telemetry_test

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/sousair/gocore/pkg/telemetry"
)

func TestTracerFromContextAndRecordError(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	ctx, parent := tp.Tracer("root").Start(context.Background(), "parent")

	t.Run("given ctx with span/when child span errors/then status error and same trace", func(t *testing.T) {
		tracer := telemetry.TracerFromContext(ctx, "scope/test")
		cctx, span := tracer.Start(ctx, "Usecase.Op")
		telemetry.RecordError(span, errors.New("boom"), "boom")
		span.End()
		_ = cctx
		spans := rec.Ended()
		if len(spans) != 1 {
			t.Fatalf("want 1 ended span, got %d", len(spans))
		}
		s := spans[0]
		if s.Status().Code != codes.Error {
			t.Fatalf("want error status, got %v", s.Status().Code)
		}
		if s.SpanContext().TraceID() != parent.SpanContext().TraceID() {
			t.Fatal("child span not in parent trace")
		}
		if len(s.Events()) == 0 || s.Events()[0].Name != "exception" {
			t.Fatal("exception event not recorded")
		}
	})
	parent.End()

	t.Run("given nil span or nil err/when RecordError/then no panic", func(t *testing.T) {
		telemetry.RecordError(nil, nil, "x")
	})
}
