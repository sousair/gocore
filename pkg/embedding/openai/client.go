// Package openai provides an embedding client for any OpenAI-compatible
// /v1/embeddings endpoint (OpenAI itself, or a self-hosted llama.cpp server)
// that implements the embedding.Embedder interface.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sousair/gocore/pkg/embedding"
	"github.com/sousair/gocore/pkg/telemetry"
)

// Dims is the vector length every returned embedding must have, matching the
// bge-m3 vectors stored by consumers.
const Dims = 1024

const providerName = "openai-compatible"

var _ embedding.Embedder = (*Client)(nil)

// Dependencies configures a Client. BaseURL excludes the /v1 suffix (e.g.
// "http://embeddings.data.svc.cluster.local"). APIKey is sent as a bearer
// token only when set; HTTPClient defaults to a 30s-timeout client.
type Dependencies struct {
	BaseURL    string
	Model      string
	APIKey     string
	HTTPClient *http.Client
}

type Client struct {
	baseURL string
	model   string
	apiKey  string
	http    *http.Client
}

func New(deps *Dependencies) *Client {
	c := &Client{baseURL: deps.BaseURL, model: deps.Model, apiKey: deps.APIKey, http: deps.HTTPClient}
	if c.http == nil {
		c.http = &http.Client{Timeout: 30 * time.Second}
	}
	return c
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Model string `json:"model"`
	Data  []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
	} `json:"usage"`
}

// Embed returns a Dims-length embedding for text.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	vecs, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

// EmbedBatch embeds texts in one call and returns their vectors in input order.
func (c *Client) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	ctx, span := telemetry.StartLLMCall(ctx, telemetry.LLMCallInfo{
		Operation: "embeddings", Provider: providerName, RequestModel: c.model,
	})
	out, err := c.call(ctx, texts)
	var vecs [][]float32
	if err == nil {
		vecs, err = byIndex(out, len(texts))
	}
	span.End(telemetry.LLMResult{ResponseModel: out.Model, InputTokens: out.Usage.PromptTokens}, err)
	return vecs, err
}

func (c *Client) call(ctx context.Context, texts []string) (embedResponse, error) {
	body, err := json.Marshal(embedRequest{Model: c.model, Input: texts})
	if err != nil {
		return embedResponse{}, fmt.Errorf("openai: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return embedResponse{}, fmt.Errorf("openai: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return embedResponse{}, fmt.Errorf("openai: call: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return embedResponse{}, fmt.Errorf("openai: status %d", res.StatusCode)
	}

	var out embedResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return embedResponse{}, fmt.Errorf("openai: decode: %w", err)
	}
	return out, nil
}

func byIndex(out embedResponse, want int) ([][]float32, error) {
	if len(out.Data) != want {
		return nil, fmt.Errorf("openai: want %d vectors, got %d", want, len(out.Data))
	}
	vecs := make([][]float32, want)
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= want || vecs[d.Index] != nil {
			return nil, fmt.Errorf("openai: invalid or duplicate index %d", d.Index)
		}
		if len(d.Embedding) != Dims {
			return nil, fmt.Errorf("openai: want %d dims, got %d", Dims, len(d.Embedding))
		}
		vecs[d.Index] = d.Embedding
	}
	return vecs, nil
}
