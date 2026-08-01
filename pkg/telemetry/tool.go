package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ToolCallInfo describes an agent tool execution. ToolType is one of
// function | extension | datastore.
type ToolCallInfo struct {
	Name        string
	CallID      string
	Description string
	ToolType    string
	Round       int
}

// ToolCallResult is the outcome. A tool that rejected the model's arguments is
// OK=false with an ErrorCode — a model-misuse signal, not a system failure, so it
// sets error.type without setting span status Error. Keeping the two apart is what
// lets "how often does the model call tools wrongly" be queryable without the
// service's error rate drowning in it.
type ToolCallResult struct {
	OK        bool
	ErrorCode string
}

type ToolSpan struct {
	ctx   context.Context
	span  trace.Span
	info  ToolCallInfo
	start time.Time
}

func StartToolCall(ctx context.Context, info ToolCallInfo) (context.Context, *ToolSpan) {
	attrs := []attribute.KeyValue{
		attribute.String(AttrGenAIOperationName, OpExecuteTool),
		attribute.String(AttrGenAIToolName, info.Name),
	}
	if info.CallID != "" {
		attrs = append(attrs, attribute.String(AttrGenAIToolCallID, info.CallID))
	}
	if info.Description != "" {
		attrs = append(attrs, attribute.String(AttrGenAIToolDescription, info.Description))
	}
	if info.ToolType != "" {
		attrs = append(attrs, attribute.String(AttrGenAIToolType, info.ToolType))
	}
	if info.Round > 0 {
		attrs = append(attrs, attribute.Int(AttrGenAIRound, info.Round))
	}

	tracer := TracerFromContext(ctx, llmScope)
	ctx, span := tracer.Start(ctx, OpExecuteTool+" "+info.Name, trace.WithAttributes(attrs...))
	return ctx, &ToolSpan{ctx: ctx, span: span, info: info, start: time.Now()}
}

func (s *ToolSpan) End(res ToolCallResult, err error) {
	dur := time.Since(s.start)

	switch {
	case err != nil:
		s.span.SetAttributes(attribute.String(AttrErrorType, "tool.execution_failed"))
		RecordError(s.span, err, err.Error())
		slog.ErrorContext(s.ctx,
			fmt.Sprintf("genai.tool %s failed: %s", s.info.Name, err.Error()),
			Err(err),
			slog.String("event", "genai.tool"),
			slog.String("tool", s.info.Name),
			slog.Int64("duration_ms", dur.Milliseconds()))

	case !res.OK:
		if res.ErrorCode != "" {
			s.span.SetAttributes(attribute.String(AttrErrorType, res.ErrorCode))
		}
		slog.WarnContext(s.ctx,
			fmt.Sprintf("genai.tool %s rejected: %s", s.info.Name, res.ErrorCode),
			slog.String("event", "genai.tool"),
			slog.String("tool", s.info.Name),
			slog.String("error_code", res.ErrorCode),
			slog.Int64("duration_ms", dur.Milliseconds()))

	default:
		slog.InfoContext(s.ctx,
			fmt.Sprintf("genai.tool %s ok %s", s.info.Name, dur.Round(time.Millisecond)),
			slog.String("event", "genai.tool"),
			slog.String("tool", s.info.Name),
			slog.Int64("duration_ms", dur.Milliseconds()))
	}

	s.span.End()
}
