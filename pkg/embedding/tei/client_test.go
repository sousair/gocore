package tei_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sousair/gocore/pkg/embedding/tei"
)

func makeVector(dim int) []float32 {
	v := make([]float32, dim)
	for i := range v {
		v[i] = float32(i) * 0.001
	}
	return v
}

func teiServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestClient_Embed_HappyPath(t *testing.T) {
	vec := makeVector(tei.Dims)
	srv := teiServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/embed" {
			http.Error(w, "wrong endpoint", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([][]float32{vec}) //nolint:errcheck
	})

	client := tei.NewClient(srv.URL)
	got, err := client.Embed(context.Background(), "FII dividends Q1")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(got) != tei.Dims {
		t.Errorf("want %d dims, got %d", tei.Dims, len(got))
	}
	if got[0] != vec[0] || got[tei.Dims-1] != vec[tei.Dims-1] {
		t.Error("vector mismatch")
	}
}

func TestClient_Embed_WrongDims(t *testing.T) {
	srv := teiServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([][]float32{makeVector(512)}) //nolint:errcheck
	})

	client := tei.NewClient(srv.URL)
	_, err := client.Embed(context.Background(), "test")
	if err == nil {
		t.Error("want error for 512-dim vector, got nil")
	}
}

func TestClient_Embed_NonOKStatus(t *testing.T) {
	srv := teiServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model loading", http.StatusServiceUnavailable)
	})

	client := tei.NewClient(srv.URL)
	_, err := client.Embed(context.Background(), "test")
	if err == nil {
		t.Error("want error for 503 response, got nil")
	}
}

func TestClient_Embed_EmptyResponse(t *testing.T) {
	srv := teiServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([][]float32{}) //nolint:errcheck
	})

	client := tei.NewClient(srv.URL)
	_, err := client.Embed(context.Background(), "test")
	if err == nil {
		t.Error("want error for empty response, got nil")
	}
}
