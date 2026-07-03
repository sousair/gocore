package telemetry

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// traceHandler is slog middleware that stamps every record produced inside an
// active span with the flat trace_id / span_id fields Grafana derived fields
// and the otelslog ecosystem expect.
type traceHandler struct{ inner slog.Handler }

func newTraceHandler(inner slog.Handler) slog.Handler { return &traceHandler{inner: inner} }

func (h *traceHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *traceHandler) Handle(ctx context.Context, rec slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		rec = rec.Clone()
		rec.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.inner.Handle(ctx, rec)
}

func (h *traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *traceHandler) WithGroup(name string) slog.Handler {
	return &traceHandler{inner: h.inner.WithGroup(name)}
}

// fanoutHandler delivers each record to every sink; used to pair the stdout
// JSON handler with the optional OTLP bridge.
type fanoutHandler struct{ sinks []slog.Handler }

func newFanoutHandler(hs ...slog.Handler) slog.Handler { return &fanoutHandler{sinks: hs} }

func (h *fanoutHandler) Enabled(ctx context.Context, l slog.Level) bool {
	for _, s := range h.sinks {
		if s.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (h *fanoutHandler) Handle(ctx context.Context, rec slog.Record) error {
	var firstErr error
	for _, s := range h.sinks {
		if s.Enabled(ctx, rec.Level) {
			if err := s.Handle(ctx, rec.Clone()); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (h *fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Handler, len(h.sinks))
	for i, s := range h.sinks {
		out[i] = s.WithAttrs(attrs)
	}
	return &fanoutHandler{sinks: out}
}

func (h *fanoutHandler) WithGroup(name string) slog.Handler {
	out := make([]slog.Handler, len(h.sinks))
	for i, s := range h.sinks {
		out[i] = s.WithGroup(name)
	}
	return &fanoutHandler{sinks: out}
}

// levelHandler gates an inner handler by a minimum level. It exists because
// sdk/log (and the otelslog bridge built on top of it) has no min-severity
// construction option, unlike slog.HandlerOptions.Level on the stdout
// handler — without this wrapper the OTLP log path would ignore LOG_LEVEL
// and ship every record regardless of severity.
type levelHandler struct {
	min   slog.Level
	inner slog.Handler
}

func newLevelHandler(min slog.Level, inner slog.Handler) slog.Handler {
	return &levelHandler{min: min, inner: inner}
}

func (h *levelHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= h.min && h.inner.Enabled(ctx, l)
}

func (h *levelHandler) Handle(ctx context.Context, rec slog.Record) error {
	return h.inner.Handle(ctx, rec)
}

func (h *levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &levelHandler{min: h.min, inner: h.inner.WithAttrs(attrs)}
}

func (h *levelHandler) WithGroup(name string) slog.Handler {
	return &levelHandler{min: h.min, inner: h.inner.WithGroup(name)}
}
