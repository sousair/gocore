package telemetry_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sousair/gocore/pkg/telemetry"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func attrsOf(t *testing.T, s sdktrace.ReadOnlySpan) map[string]any {
	t.Helper()
	out := map[string]any{}
	for _, kv := range s.Attributes() {
		out[string(kv.Key)] = kv.Value.AsInterface()
	}
	return out
}

func TestStartToolCall(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	ctx, _ := tp.Tracer("t").Start(context.Background(), "root")

	t.Run("given a successful call/when End/then the span is named execute_tool <name> with semconv attrs", func(t *testing.T) {
		_, ts := telemetry.StartToolCall(ctx, telemetry.ToolCallInfo{
			Name: "list_accounts", CallID: "toolu_1", Description: "List accounts", ToolType: "function", Round: 2,
		})
		ts.End(telemetry.ToolCallResult{OK: true}, nil)

		spans := rec.Ended()
		last := spans[len(spans)-1]
		if last.Name() != "execute_tool list_accounts" {
			t.Fatalf("span name = %q, want %q", last.Name(), "execute_tool list_accounts")
		}
		a := attrsOf(t, last)
		if a["gen_ai.operation.name"] != "execute_tool" {
			t.Errorf("operation.name = %v", a["gen_ai.operation.name"])
		}
		if a["gen_ai.tool.name"] != "list_accounts" || a["gen_ai.tool.call.id"] != "toolu_1" {
			t.Errorf("tool attrs = %v", a)
		}
		if a["gen_ai.tool.type"] != "function" || a["gen_ai.round"] != int64(2) {
			t.Errorf("type/round = %v", a)
		}
		if last.Status().Code == codes.Error {
			t.Error("a successful call must not set span status Error")
		}
	})

	t.Run("given a tool rejection/when End/then error.type is set but span status stays unset", func(t *testing.T) {
		_, ts := telemetry.StartToolCall(ctx, telemetry.ToolCallInfo{Name: "resolve_pending", ToolType: "function"})
		ts.End(telemetry.ToolCallResult{OK: false, ErrorCode: "schema.invalid"}, nil)

		spans := rec.Ended()
		last := spans[len(spans)-1]
		if attrsOf(t, last)["error.type"] != "schema.invalid" {
			t.Errorf("error.type = %v, want schema.invalid", attrsOf(t, last)["error.type"])
		}
		if last.Status().Code == codes.Error {
			t.Error("a model misusing a tool is not a system failure — status must stay unset")
		}
	})

	t.Run("given an infrastructure error/when End/then span status is Error", func(t *testing.T) {
		_, ts := telemetry.StartToolCall(ctx, telemetry.ToolCallInfo{Name: "list_accounts", ToolType: "function"})
		ts.End(telemetry.ToolCallResult{}, errors.New("pool closed"))

		spans := rec.Ended()
		if spans[len(spans)-1].Status().Code != codes.Error {
			t.Error("want span status Error")
		}
	})

	t.Run("given no round/when End/then the round attribute is omitted", func(t *testing.T) {
		_, ts := telemetry.StartToolCall(ctx, telemetry.ToolCallInfo{Name: "list_tags", ToolType: "function"})
		ts.End(telemetry.ToolCallResult{OK: true}, nil)

		spans := rec.Ended()
		if _, ok := attrsOf(t, spans[len(spans)-1])["gen_ai.round"]; ok {
			t.Error("round 0 must not be emitted")
		}
	})
}
