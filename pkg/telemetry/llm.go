package telemetry

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const llmScope = "github.com/sousair/gocore/pkg/telemetry"

type LLMCallInfo struct{ Operation, Provider, RequestModel string }

type LLMResult struct {
	ResponseModel string
	InputTokens   int
	OutputTokens  int
}

type LLMSpan struct {
	ctx   context.Context
	span  trace.Span
	info  LLMCallInfo
	start time.Time
}

func StartLLMCall(ctx context.Context, info LLMCallInfo) (context.Context, *LLMSpan) {
	tracer := TracerFromContext(ctx, llmScope)
	ctx, span := tracer.Start(ctx, "gen_ai."+info.Operation, trace.WithAttributes(
		attribute.String(AttrGenAIOperationName, info.Operation),
		attribute.String(AttrGenAIProviderName, info.Provider),
		attribute.String(AttrGenAIRequestModel, info.RequestModel),
	))
	return ctx, &LLMSpan{ctx: ctx, span: span, info: info, start: time.Now()}
}

func (s *LLMSpan) End(res LLMResult, err error) {
	dur := time.Since(s.start)
	if res.ResponseModel != "" {
		s.span.SetAttributes(attribute.String(AttrGenAIResponseModel, res.ResponseModel))
	}
	s.span.SetAttributes(
		attribute.Int(AttrGenAIInputTokens, res.InputTokens),
		attribute.Int(AttrGenAIOutputTokens, res.OutputTokens),
	)
	if err != nil {
		RecordError(s.span, err, err.Error())
		slog.ErrorContext(s.ctx, "genai.call", Err(err),
			slog.String("operation", s.info.Operation), slog.String("provider", s.info.Provider),
			slog.String("request_model", s.info.RequestModel), slog.Int64("duration_ms", dur.Milliseconds()))
	} else {
		slog.InfoContext(s.ctx, "genai.call",
			"operation", s.info.Operation, "provider", s.info.Provider,
			"request_model", s.info.RequestModel, "response_model", res.ResponseModel,
			"input_tokens", res.InputTokens, "output_tokens", res.OutputTokens,
			"duration_ms", dur.Milliseconds())
	}
	s.span.End()
}
