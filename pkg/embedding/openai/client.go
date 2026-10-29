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
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
	} `json:"usage"`
}

// Embed returns a Dims-length embedding for text.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	ctx, span := telemetry.StartLLMCall(ctx, telemetry.LLMCallInfo{
		Operation: "embeddings", Provider: providerName, RequestModel: c.model,
	})
	out, err := c.call(ctx, text)
	if err == nil && (len(out.Data) != 1 || len(out.Data[0].Embedding) != Dims) {
		err = fmt.Errorf("openai: want 1x%d vector, got %dx%d", Dims, len(out.Data), dimsOf(out))
	}
	span.End(telemetry.LLMResult{ResponseModel: out.Model, InputTokens: out.Usage.PromptTokens}, err)
	if err != nil {
		return nil, err
	}
	return out.Data[0].Embedding, nil
}

func (c *Client) call(ctx context.Context, text string) (embedResponse, error) {
	body, err := json.Marshal(embedRequest{Model: c.model, Input: []string{text}})
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

func dimsOf(out embedResponse) int {
	if len(out.Data) == 0 {
		return 0
	}
	return len(out.Data[0].Embedding)
}
