// Package tei provides a TEI (text-embeddings-inference) client that
// implements the embedding.Embedder interface using the bge-m3 model
// (1024-dim, multilingual).
package tei

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

// compile-time check that Client satisfies the Embedder port.
var _ embedding.Embedder = (*Client)(nil)

// Client calls a self-hosted TEI server and implements embedding.Embedder.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient constructs a Client that sends embed requests to baseURL.
// baseURL should not include a trailing slash (e.g., "http://tei:8080").
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Embed returns a 1024-dimensional embedding for text by calling the TEI
// /embed endpoint. The returned slice always has length Dims on
// success.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(map[string]any{"inputs": []string{text}})
	if err != nil {
		return nil, fmt.Errorf("tei: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("tei: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tei: call: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tei: status %d", res.StatusCode)
	}

	var out [][]float32
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("tei: decode: %w", err)
	}
	if len(out) != 1 || len(out[0]) != Dims {
		return nil, fmt.Errorf("tei: want 1x%d vector, got %dx%d", Dims, len(out), dimsOf(out))
	}
	return out[0], nil
}

func dimsOf(out [][]float32) int {
	if len(out) == 0 {
		return 0
	}
	return len(out[0])
}
