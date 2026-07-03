package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestErr(t *testing.T) {
	t.Run("given error/when attr/then key error and message value", func(t *testing.T) {
		a := Err(errors.New("boom"))
		if a.Key != "error" || a.Value.String() != "boom" {
			t.Fatalf("got %s=%s", a.Key, a.Value.String())
		}
	})
	t.Run("given nil/when attr/then <nil> value", func(t *testing.T) {
		if got := Err(nil).Value.String(); got != "<nil>" {
			t.Fatalf("got %q", got)
		}
	})
}

func logLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("unmarshal %q: %v", buf.String(), err)
	}
	return m
}

func TestTraceHandler(t *testing.T) {
	t.Run("given active span/when log/then trace_id and span_id injected", func(t *testing.T) {
		buf := &bytes.Buffer{}
		log := slog.New(newTraceHandler(slog.NewJSONHandler(buf, nil)))
		tp := sdktrace.NewTracerProvider() // no exporter: spans are real but unshipped
		ctx, span := tp.Tracer("test").Start(context.Background(), "op")
		log.InfoContext(ctx, "hello")
		span.End()
		m := logLine(t, buf)
		if m["trace_id"] != span.SpanContext().TraceID().String() {
			t.Fatalf("trace_id missing or wrong: %v", m["trace_id"])
		}
		if m["span_id"] == nil {
			t.Fatal("span_id missing")
		}
	})
	t.Run("given no span/when log/then no trace fields", func(t *testing.T) {
		buf := &bytes.Buffer{}
		log := slog.New(newTraceHandler(slog.NewJSONHandler(buf, nil)))
		log.InfoContext(context.Background(), "hello")
		if m := logLine(t, buf); m["trace_id"] != nil {
			t.Fatal("unexpected trace_id")
		}
	})
}

func TestLevelHandler(t *testing.T) {
	t.Run("given min warn/when debug record/then suppressed", func(t *testing.T) {
		buf := &bytes.Buffer{}
		log := slog.New(newLevelHandler(slog.LevelWarn, slog.NewJSONHandler(buf, nil)))
		log.DebugContext(context.Background(), "hello")
		if buf.Len() != 0 {
			t.Fatalf("expected no output, got %q", buf.String())
		}
	})
	t.Run("given min warn/when info record/then suppressed", func(t *testing.T) {
		buf := &bytes.Buffer{}
		log := slog.New(newLevelHandler(slog.LevelWarn, slog.NewJSONHandler(buf, nil)))
		log.InfoContext(context.Background(), "hello")
		if buf.Len() != 0 {
			t.Fatalf("expected no output, got %q", buf.String())
		}
	})
	t.Run("given min warn/when warn record/then passes", func(t *testing.T) {
		buf := &bytes.Buffer{}
		log := slog.New(newLevelHandler(slog.LevelWarn, slog.NewJSONHandler(buf, nil)))
		log.WarnContext(context.Background(), "hello")
		if buf.Len() == 0 {
			t.Fatal("expected output, got none")
		}
	})
}

func TestFanoutHandler(t *testing.T) {
	t.Run("given two sinks/when log/then both receive record", func(t *testing.T) {
		a, b := &bytes.Buffer{}, &bytes.Buffer{}
		log := slog.New(newFanoutHandler(slog.NewJSONHandler(a, nil), slog.NewJSONHandler(b, nil)))
		log.InfoContext(context.Background(), "x")
		if a.Len() == 0 || b.Len() == 0 {
			t.Fatal("fanout did not reach both sinks")
		}
	})
}
