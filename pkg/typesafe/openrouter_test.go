package typesafe

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sousair/gocore/pkg/telemetry"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestSystemOne_openRouterRequest(t *testing.T) {
	var body map[string]any
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(okBody))
	}))
	t.Cleanup(srv.Close)
	c := New(&Dependencies{
		APIKey: "k", Model: OpenRouterModel, Endpoint: srv.URL + "/api/alpha/decisions",
		Provider: map[string]any{"data_collection": "deny", "zdr": true},
	})

	if _, err := c.SystemOne(context.Background(), nil, questions()); err != nil {
		t.Fatalf("SystemOne: %v", err)
	}
	provider, _ := body["provider"].(map[string]any)
	if path != "/api/alpha/decisions" || body["model"] != OpenRouterModel ||
		provider["data_collection"] != "deny" || provider["zdr"] != true {
		t.Errorf("path=%s body=%v", path, body)
	}
}

func TestSystemOne_providerOmittedWhenNil(t *testing.T) {
	var body map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(okBody))
	})
	if _, err := c.SystemOne(context.Background(), nil, questions()); err != nil {
		t.Fatalf("SystemOne: %v", err)
	}
	if _, ok := body["provider"]; ok {
		t.Errorf("provider sent: %v", body)
	}
}

func TestSystemOne_spanCostAndProvider(t *testing.T) {
	cases := []struct {
		name, usage, endpoint, provider string
		cost                            float64
	}{
		{"reported cost wins", `{"input_tokens":120,"output_tokens":0,"cost":0.25}`, OpenRouterEndpoint, "openrouter", 0.25},
		{"token price fallback", `{"input_tokens":1000000,"output_tokens":0}`, "", "typesafe", 0.042},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"model":"m","answers":{},"usage":` + tc.usage + `}`))
			}))
			t.Cleanup(srv.Close)
			c := New(&Dependencies{APIKey: "k", Model: "m", Endpoint: tc.endpoint})
			c.endpoint = srv.URL // keep the provider derived from the configured endpoint

			rec := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
			ctx, root := tp.Tracer("t").Start(context.Background(), "root")
			if _, err := c.SystemOne(ctx, nil, questions()); err != nil {
				t.Fatalf("SystemOne: %v", err)
			}
			root.End()

			attrs := map[string]any{}
			for _, kv := range rec.Ended()[0].Attributes() {
				attrs[string(kv.Key)] = kv.Value.AsInterface()
			}
			cost, _ := attrs[telemetry.AttrGenAICostUSD].(float64)
			if attrs[telemetry.AttrGenAIProviderName] != tc.provider || math.Abs(cost-tc.cost) > 1e-12 {
				t.Errorf("attrs = %v", attrs)
			}
		})
	}
}

func TestSystemOne_529RetriedOnce(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(529)
			return
		}
		_, _ = w.Write([]byte(okBody))
	})
	if _, err := c.SystemOne(context.Background(), nil, questions()); err != nil || calls.Load() != 2 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestSystemOne_openRouterErrorMessage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error":{"code":402,"message":"Insufficient credits","metadata":{}}}`))
	})
	_, err := c.SystemOne(context.Background(), nil, questions())
	if err == nil || !strings.Contains(err.Error(), "status 402: Insufficient credits") {
		t.Fatalf("got %v", err)
	}
}
