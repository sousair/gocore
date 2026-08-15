package openrouter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func makeVector(dim int) []float32 {
	v := make([]float32, dim)
	for i := range v {
		v[i] = float32(i) * 0.001
	}
	return v
}

func openrouterServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func newTestClient(srv *httptest.Server) *Client {
	c := NewClient("test-key")
	c.setBaseURL(srv.URL)
	return c
}

func TestClient_Embed_HappyPath(t *testing.T) {
	vec := makeVector(Dims)
	srv := openrouterServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(embedResponse{ //nolint:errcheck
			Data: []struct {
				Embedding []float32 `json:"embedding"`
			}{{Embedding: vec}},
		})
	})

	client := newTestClient(srv)
	got, err := client.Embed(context.Background(), "FII dividends Q1")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(got) != Dims {
		t.Errorf("want %d dims, got %d", Dims, len(got))
	}
	if got[0] != vec[0] || got[Dims-1] != vec[Dims-1] {
		t.Error("vector mismatch")
	}
}

func TestClient_Embed_RequestShape(t *testing.T) {
	vec := makeVector(Dims)
	var gotBody map[string]any
	var gotAuth string
	srv := openrouterServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(embedResponse{ //nolint:errcheck
			Data: []struct {
				Embedding []float32 `json:"embedding"`
			}{{Embedding: vec}},
		})
	})

	client := newTestClient(srv)
	if _, err := client.Embed(context.Background(), "FII dividends Q1"); err != nil {
		t.Fatalf("Embed: %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Errorf("want Authorization %q, got %q", "Bearer test-key", gotAuth)
	}
	if gotBody["model"] != "baai/bge-m3" {
		t.Errorf("want model %q, got %v", "baai/bge-m3", gotBody["model"])
	}
	if gotBody["input"] != "FII dividends Q1" {
		t.Errorf("want input as plain string, got %v", gotBody["input"])
	}
	provider, ok := gotBody["provider"].(map[string]any)
	if !ok {
		t.Fatalf("want provider object, got %v", gotBody["provider"])
	}
	order, ok := provider["order"].([]any)
	if !ok || len(order) != 2 || order[0] != "DeepInfra" || order[1] != "Parasail" {
		t.Errorf("want provider.order [DeepInfra Parasail], got %v", provider["order"])
	}
}

func TestClient_Embed_NonOKStatus(t *testing.T) {
	srv := openrouterServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	})

	client := newTestClient(srv)
	_, err := client.Embed(context.Background(), "test")
	if err == nil {
		t.Error("want error for 429 response, got nil")
	}
}

func TestClient_Embed_WrongDims(t *testing.T) {
	srv := openrouterServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(embedResponse{ //nolint:errcheck
			Data: []struct {
				Embedding []float32 `json:"embedding"`
			}{{Embedding: makeVector(512)}},
		})
	})

	client := newTestClient(srv)
	_, err := client.Embed(context.Background(), "test")
	if err == nil {
		t.Error("want error for 512-dim vector, got nil")
	}
}

func TestClient_Embed_EmptyData(t *testing.T) {
	srv := openrouterServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(embedResponse{}) //nolint:errcheck
	})

	client := newTestClient(srv)
	_, err := client.Embed(context.Background(), "test")
	if err == nil {
		t.Error("want error for empty data, got nil")
	}
}
