package telemetry

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// TraceInjectingTransport adds OpenRouter's `trace` object to outbound
// chat-completion bodies so OpenRouter's generation span parents onto the
// caller's span instead of arriving as a disconnected root. OpenRouter uses a
// supplied 32-hex trace id and 16-hex span id verbatim; anything else it
// re-derives, which is why only valid W3C ids are ever sent.
//
// go-openai (v1.41.2) exposes no extra-body hook — ClientConfig has no such
// field and withExtraBody is unexported — so the injection happens here rather
// than at the call site.
type TraceInjectingTransport struct{ Base http.RoundTripper }

func NewTraceInjectingTransport(base http.RoundTripper) *TraceInjectingTransport {
	return &TraceInjectingTransport{Base: base}
}

func (t *TraceInjectingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	sc := trace.SpanContextFromContext(req.Context())
	if !eligible(req) || !sc.IsValid() {
		return base.RoundTrip(req)
	}

	original, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return nil, err
	}

	patched, err := withTraceObject(original, sc.TraceID().String(), sc.SpanID().String())
	if err != nil {
		// A body we cannot parse is not ours to rewrite; send it as-is and let
		// the API report whatever is actually wrong with it.
		patched = original
	}

	clone := req.Clone(req.Context())
	clone.Body = io.NopCloser(bytes.NewReader(patched))
	clone.ContentLength = int64(len(patched))
	// req.Clone shallow-copies GetBody, which still closes over the original
	// pre-injection body. A redirect resend or HTTP/2 retry that calls it would
	// silently drop the trace object, so it must be rebound to the patched bytes.
	clone.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(patched)), nil
	}
	return base.RoundTrip(clone)
}

func eligible(req *http.Request) bool {
	return req.Body != nil &&
		req.Method == http.MethodPost &&
		strings.HasSuffix(req.URL.Path, "/chat/completions")
}

// withTraceObject fills trace_id and parent_span_id without disturbing keys the
// caller already set — a caller-supplied trace_id is a deliberate grouping
// choice and outranks ours.
func withTraceObject(body []byte, traceID, spanID string) ([]byte, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	existing := map[string]any{}
	if raw, ok := payload["trace"]; ok {
		if err := json.Unmarshal(raw, &existing); err != nil {
			return nil, err
		}
	}
	if _, ok := existing["trace_id"]; !ok {
		existing["trace_id"] = traceID
	}
	if _, ok := existing["parent_span_id"]; !ok {
		existing["parent_span_id"] = spanID
	}

	merged, err := json.Marshal(existing)
	if err != nil {
		return nil, err
	}
	payload["trace"] = merged

	return json.Marshal(payload)
}
