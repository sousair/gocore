package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/sousair/gocore/pkg/telemetry"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestStartLLMCall(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	ctx, root := tp.Tracer("t").Start(context.Background(), "root")

	t.Run("given success/when End/then span has genai attrs and ok status", func(t *testing.T) {
		cctx, ls := telemetry.StartLLMCall(ctx, telemetry.LLMCallInfo{
			Operation: "chat", Provider: "openrouter", RequestModel: "gpt-x",
		})
		_ = cctx
		ls.End(telemetry.LLMResult{ResponseModel: "gpt-x-1", InputTokens: 10, OutputTokens: 20}, nil)
		spans := rec.Ended()
		if len(spans) != 1 || spans[0].Name() != "gen_ai.chat" {
			t.Fatalf("spans: %+v", spans)
		}
		attrs := map[string]any{}
		for _, kv := range spans[0].Attributes() {
			attrs[string(kv.Key)] = kv.Value.AsInterface()
		}
		if attrs["gen_ai.usage.input_tokens"] != int64(10) || attrs["gen_ai.response.model"] != "gpt-x-1" {
			t.Fatalf("attrs: %v", attrs)
		}
	})
	t.Run("given error/when End/then span status error", func(t *testing.T) {
		_, ls := telemetry.StartLLMCall(ctx, telemetry.LLMCallInfo{Operation: "chat", Provider: "openrouter", RequestModel: "m"})
		ls.End(telemetry.LLMResult{}, errors.New("rate limited"))
		spans := rec.Ended()
		if spans[len(spans)-1].Status().Code != codes.Error {
			t.Fatal("want error status")
		}
	})
	t.Run("given success/when End/then genai.call log carries event attr and enriched message", func(t *testing.T) {
		buf := &bytes.Buffer{}
		prev := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(buf, nil)))
		defer slog.SetDefault(prev)

		_, ls := telemetry.StartLLMCall(ctx, telemetry.LLMCallInfo{Operation: "chat", Provider: "openrouter", RequestModel: "m"})
		ls.End(telemetry.LLMResult{ResponseModel: "m-1", InputTokens: 12, OutputTokens: 3}, nil)

		var logRec map[string]any
		if err := json.Unmarshal(buf.Bytes(), &logRec); err != nil {
			t.Fatalf("genai log not json: %q", buf.String())
		}
		if logRec["event"] != "genai.call" {
			t.Fatalf("want event=genai.call, got %v", logRec)
		}
		if logRec["input_tokens"] != float64(12) || logRec["output_tokens"] != float64(3) {
			t.Fatalf("want token attrs preserved, got %v", logRec)
		}
		msg, _ := logRec["msg"].(string)
		if !strings.Contains(msg, "genai.call chat openrouter") {
			t.Fatalf("want enriched genai message, got %q", msg)
		}
	})
	root.End()
}

func TestLLMSpanEmitsReasoningAttributes(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	ctx, root := tp.Tracer("t").Start(context.Background(), "root")

	_, ls := telemetry.StartLLMCall(ctx, telemetry.LLMCallInfo{
		Operation: "chat", Provider: "openrouter",
		RequestModel: "anthropic/claude-sonnet-5", FlowName: "agent",
	})
	ls.End(telemetry.LLMResult{
		ResponseModel: "anthropic/claude-sonnet-5",
		InputTokens:   100, OutputTokens: 700,
		ReasoningTokens: 700, FinishReason: "length",
	}, nil)
	root.End()

	attrs := map[string]any{}
	for _, kv := range rec.Ended()[0].Attributes() {
		attrs[string(kv.Key)] = kv.Value.AsInterface()
	}
	if got := attrs[telemetry.AttrGenAIFlowName]; got != "agent" {
		t.Errorf("flow name = %v, want agent", got)
	}
	if got := attrs[telemetry.AttrGenAIFinishReason]; got != "length" {
		t.Errorf("finish reason = %v, want length", got)
	}
	if got := attrs[telemetry.AttrGenAIReasoningTokens]; got != int64(700) {
		t.Errorf("reasoning tokens = %v, want 700", got)
	}
}

func TestLLMCacheTokens(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	ctx, _ := tp.Tracer("t").Start(context.Background(), "root")

	t.Run("given cache counts/when End/then span carries both attrs", func(t *testing.T) {
		_, ls := telemetry.StartLLMCall(ctx, telemetry.LLMCallInfo{
			Operation: "chat", Provider: "openrouter", RequestModel: "m",
		})
		ls.End(telemetry.LLMResult{
			ResponseModel: "m-1", InputTokens: 24790,
			CachedInputTokens: 6785, CacheWriteTokens: 18005,
		}, nil)

		spans := rec.Ended()
		attrs := map[string]any{}
		for _, kv := range spans[len(spans)-1].Attributes() {
			attrs[string(kv.Key)] = kv.Value.AsInterface()
		}
		if attrs["gen_ai.usage.cached_input_tokens"] != int64(6785) {
			t.Fatalf("cached: %v", attrs)
		}
		if attrs["gen_ai.usage.cache_write_tokens"] != int64(18005) {
			t.Fatalf("write: %v", attrs)
		}
	})

	t.Run("given no cache counts/when End/then attrs absent", func(t *testing.T) {
		_, ls := telemetry.StartLLMCall(ctx, telemetry.LLMCallInfo{
			Operation: "chat", Provider: "openrouter", RequestModel: "m",
		})
		ls.End(telemetry.LLMResult{ResponseModel: "m-1", InputTokens: 10}, nil)

		spans := rec.Ended()
		for _, kv := range spans[len(spans)-1].Attributes() {
			if strings.HasPrefix(string(kv.Key), "gen_ai.usage.cache") {
				t.Fatalf("unset cache count emitted: %s", kv.Key)
			}
		}
	})

	t.Run("given cache counts/when End/then genai.call log carries them", func(t *testing.T) {
		buf := &bytes.Buffer{}
		prev := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(buf, nil)))
		defer slog.SetDefault(prev)

		_, ls := telemetry.StartLLMCall(ctx, telemetry.LLMCallInfo{
			Operation: "chat", Provider: "openrouter", RequestModel: "m",
		})
		ls.End(telemetry.LLMResult{ResponseModel: "m-1", CachedInputTokens: 6785}, nil)

		var logRec map[string]any
		if err := json.Unmarshal(buf.Bytes(), &logRec); err != nil {
			t.Fatalf("genai log not json: %q", buf.String())
		}
		if logRec["cached_input_tokens"] != float64(6785) {
			t.Fatalf("log: %v", logRec)
		}
	})
}

func TestLLMSpanOmitsUnsetReasoningAttributes(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	ctx, root := tp.Tracer("t").Start(context.Background(), "root")

	_, ls := telemetry.StartLLMCall(ctx, telemetry.LLMCallInfo{
		Operation: "chat", Provider: "openrouter", RequestModel: "m",
	})
	ls.End(telemetry.LLMResult{InputTokens: 1, OutputTokens: 2}, nil)
	root.End()

	for _, kv := range rec.Ended()[0].Attributes() {
		switch string(kv.Key) {
		case telemetry.AttrGenAIFlowName, telemetry.AttrGenAIFinishReason:
			t.Errorf("unset field emitted attribute %s", kv.Key)
		}
	}
}
