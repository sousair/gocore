// Package openrouter provides an OpenRouter-hosted embedding client that
// implements the embedding.Embedder interface using the baai/bge-m3 model
// (1024-dim, multilingual) — the same model self-hosted TEI deployments run,
// so vectors are interchangeable.
package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sousair/gocore/pkg/embedding"
)

// Dims is the vector length produced by the bge-m3 model this client targets.
const Dims = 1024

const defaultBaseURL = "https://openrouter.ai/api/v1"
const model = "baai/bge-m3"

// compile-time check that Client satisfies the Embedder port.
var _ embedding.Embedder = (*Client)(nil)

// Client calls OpenRouter's hosted embeddings endpoint and implements
// embedding.Embedder.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// NewClient constructs a Client authenticated with apiKey.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// setBaseURL overrides the API base URL; used by tests to point at an
// httptest server.
func (c *Client) setBaseURL(u string) {
	c.baseURL = u
}

type embedRequest struct {
	Model    string       `json:"model"`
	Input    string       `json:"input"`
	Provider providerSpec `json:"provider"`
}

// providerSpec pins bge-m3 requests to providers verified to return
// vector-compatible embeddings, rather than whatever the router picks.
// Fallbacks stay allowed (no allow_fallbacks: false) so one provider being
// down doesn't take embeddings offline.
type providerSpec struct {
	Order []string `json:"order"`
}

type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed returns a 1024-dimensional embedding for text by calling the
// OpenRouter /embeddings endpoint. The returned slice always has length
// Dims on success.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(embedRequest{
		Model: model,
		Input: text,
		Provider: providerSpec{
			Order: []string{"DeepInfra", "Parasail"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("openrouter: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openrouter: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter: call: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openrouter: status %d", res.StatusCode)
	}

	var out embedResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("openrouter: decode: %w", err)
	}
	if len(out.Data) != 1 || len(out.Data[0].Embedding) != Dims {
		return nil, fmt.Errorf("openrouter: want 1x%d vector, got %dx%d", Dims, len(out.Data), dimsOf(out))
	}
	return out.Data[0].Embedding, nil
}

func dimsOf(out embedResponse) int {
	if len(out.Data) == 0 {
		return 0
	}
	return len(out.Data[0].Embedding)
}
