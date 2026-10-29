package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func makeVector(dim int, seed float32) []float32 {
	v := make([]float32, dim)
	for i := range v {
		v[i] = seed + float32(i)*0.001
	}
	return v
}

type item struct {
	Embedding []float32 `json:"embedding"`
}

func newTestClient(t *testing.T, apiKey string, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(&Dependencies{BaseURL: srv.URL, Model: "bge-m3", APIKey: apiKey})
}

func respond(w http.ResponseWriter, items ...item) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"model": "bge-m3", "data": items}) //nolint:errcheck
}

func TestClient_Embed_HappyPath(t *testing.T) {
	vec := makeVector(Dims, 0)
	var gotPath string
	var gotBody map[string]any
	client := newTestClient(t, "", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		respond(w, item{Embedding: vec})
	})

	got, err := client.Embed(context.Background(), "FII dividends Q1")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if gotPath != "/v1/embeddings" {
		t.Errorf("want path /v1/embeddings, got %q", gotPath)
	}
	if gotBody["model"] != "bge-m3" {
		t.Errorf("want model bge-m3, got %v", gotBody["model"])
	}
	input, ok := gotBody["input"].([]any)
	if !ok || len(input) != 1 || input[0] != "FII dividends Q1" {
		t.Errorf("want input [FII dividends Q1], got %v", gotBody["input"])
	}
	if len(got) != Dims || got[Dims-1] != vec[Dims-1] {
		t.Error("vector mismatch")
	}
}

func TestClient_Embed_AuthHeader(t *testing.T) {
	cases := []struct {
		name, apiKey, want string
	}{
		{"no key sends no header", "", ""},
		{"key sends bearer", "test-key", "Bearer test-key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotAuth string
			var hasAuth bool
			client := newTestClient(t, tc.apiKey, func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				_, hasAuth = r.Header["Authorization"]
				respond(w, item{Embedding: makeVector(Dims, 0)})
			})
			if _, err := client.Embed(context.Background(), "x"); err != nil {
				t.Fatalf("Embed: %v", err)
			}
			if gotAuth != tc.want || hasAuth != (tc.want != "") {
				t.Errorf("want Authorization %q, got %q (present=%v)", tc.want, gotAuth, hasAuth)
			}
		})
	}
}

func TestClient_Embed_Errors(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"non-ok status", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}},
		{"malformed body", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("{not json"))
		}},
		{"wrong dims", func(w http.ResponseWriter, r *http.Request) {
			respond(w, item{Embedding: makeVector(512, 0)})
		}},
		{"empty data", func(w http.ResponseWriter, r *http.Request) {
			respond(w)
		}},
		{"too many vectors", func(w http.ResponseWriter, r *http.Request) {
			respond(w, item{Embedding: makeVector(Dims, 0)}, item{Embedding: makeVector(Dims, 0)})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestClient(t, "", tc.handler)
			if _, err := client.Embed(context.Background(), "x"); err == nil {
				t.Error("want error, got nil")
			}
		})
	}
}

func TestClient_Embed_Telemetry(t *testing.T) {
	cases := []struct {
		name       string
		dims       int
		wantStatus codes.Code
	}{
		{"valid response ends span ok", Dims, codes.Unset},
		{"invalid response ends span failed", 512, codes.Error},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
			ctx, root := tp.Tracer("t").Start(context.Background(), "root")
			client := newTestClient(t, "", func(w http.ResponseWriter, r *http.Request) {
				respond(w, item{Embedding: makeVector(tc.dims, 0)})
			})

			_, _ = client.Embed(ctx, "x")
			root.End()

			span := rec.Ended()[0]
			attrs := map[string]any{}
			for _, kv := range span.Attributes() {
				attrs[string(kv.Key)] = kv.Value.AsInterface()
			}
			if span.Name() != "gen_ai.embeddings" || attrs["gen_ai.provider.name"] != "openai-compatible" {
				t.Errorf("want gen_ai.embeddings span from openai-compatible, got %q %v", span.Name(), attrs)
			}
			if span.Status().Code != tc.wantStatus {
				t.Errorf("want status %v, got %v", tc.wantStatus, span.Status().Code)
			}
		})
	}
}
