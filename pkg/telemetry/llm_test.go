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
