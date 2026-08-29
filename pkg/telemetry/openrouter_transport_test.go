package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

type capturingRT struct{ body []byte }

func (c *capturingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		c.body, _ = io.ReadAll(req.Body)
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(nil)), Header: http.Header{}}, nil
}

func ctxWithSpan(t *testing.T) context.Context {
	t.Helper()
	tid, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("trace id: %v", err)
	}
	sid, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("span id: %v", err)
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: tid, SpanID: sid, TraceFlags: trace.FlagsSampled,
	})
	return trace.ContextWithSpanContext(context.Background(), sc)
}

func post(t *testing.T, ctx context.Context, url, body string, rt http.RoundTripper) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("round trip: %v", err)
	}
}

func traceObj(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	tr, ok := parsed["trace"].(map[string]any)
	if !ok {
		t.Fatalf("no trace object in body: %s", body)
	}
	return tr
}

func TestInjectsTraceContext(t *testing.T) {
	rt := &capturingRT{}
	post(t, ctxWithSpan(t), "https://openrouter.ai/api/v1/chat/completions",
		`{"model":"m","messages":[]}`, NewTraceInjectingTransport(rt))

	tr := traceObj(t, rt.body)
	if tr["trace_id"] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace_id = %v", tr["trace_id"])
	}
	if tr["parent_span_id"] != "00f067aa0ba902b7" {
		t.Errorf("parent_span_id = %v", tr["parent_span_id"])
	}
}

func TestPassesThroughWithoutSpanContext(t *testing.T) {
	rt := &capturingRT{}
	body := `{"model":"m","messages":[]}`
	post(t, context.Background(), "https://openrouter.ai/api/v1/chat/completions", body,
		NewTraceInjectingTransport(rt))

	if string(rt.body) != body {
		t.Errorf("body was modified without a span context:\n got %s\nwant %s", rt.body, body)
	}
}

func TestIgnoresNonChatCompletionPaths(t *testing.T) {
	rt := &capturingRT{}
	body := `{"input":"x"}`
	post(t, ctxWithSpan(t), "https://openrouter.ai/api/v1/embeddings", body,
		NewTraceInjectingTransport(rt))

	if string(rt.body) != body {
		t.Errorf("embeddings body was modified:\n got %s\nwant %s", rt.body, body)
	}
}

func TestDoesNotClobberCallerSuppliedTraceKeys(t *testing.T) {
	rt := &capturingRT{}
	post(t, ctxWithSpan(t), "https://openrouter.ai/api/v1/chat/completions",
		`{"model":"m","trace":{"trace_id":"caller-owned","environment":"staging"}}`,
		NewTraceInjectingTransport(rt))

	tr := traceObj(t, rt.body)
	if tr["trace_id"] != "caller-owned" {
		t.Errorf("caller trace_id overwritten: %v", tr["trace_id"])
	}
	if tr["environment"] != "staging" {
		t.Errorf("caller key lost: %v", tr["environment"])
	}
	if tr["parent_span_id"] != "00f067aa0ba902b7" {
		t.Errorf("parent_span_id not filled: %v", tr["parent_span_id"])
	}
}

func TestLeavesMalformedBodyUntouched(t *testing.T) {
	rt := &capturingRT{}
	body := `not json at all`
	post(t, ctxWithSpan(t), "https://openrouter.ai/api/v1/chat/completions", body,
		NewTraceInjectingTransport(rt))

	if string(rt.body) != body {
		t.Errorf("malformed body was altered:\n got %s\nwant %s", rt.body, body)
	}
}

// getBodyCapturingRT reads via req.GetBody() instead of req.Body, simulating a
// redirect resend or HTTP/2 retry that replays the request through GetBody.
type getBodyCapturingRT struct{ body []byte }

func (c *getBodyCapturingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.GetBody != nil {
		rc, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		c.body, _ = io.ReadAll(rc)
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(nil)), Header: http.Header{}}, nil
}

func TestGetBodyReplaysInjectedBody(t *testing.T) {
	rt := &getBodyCapturingRT{}
	post(t, ctxWithSpan(t), "https://openrouter.ai/api/v1/chat/completions",
		`{"model":"m","messages":[]}`, NewTraceInjectingTransport(rt))

	tr := traceObj(t, rt.body)
	if tr["trace_id"] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace_id = %v", tr["trace_id"])
	}
	if tr["parent_span_id"] != "00f067aa0ba902b7" {
		t.Errorf("parent_span_id = %v", tr["parent_span_id"])
	}
}
