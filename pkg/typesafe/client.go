// Package typesafe is a client for TypeSafe AI's System One API, which
// answers choice, yes/no, score and box questions about a state with
// calibrated probabilities. The text contract is also served by OpenRouter's
// decisions endpoint (OpenRouterEndpoint, OpenRouterModel). Images and Box
// are preview features (SystemOneImages); Box needs Jev 1.14.
package typesafe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sousair/gocore/pkg/telemetry"
)

const (
	TypeSafeEndpoint   = "https://api.typesafe.ai/v1/systemone"
	OpenRouterEndpoint = "https://openrouter.ai/api/alpha/decisions"
	OpenRouterModel    = "typesafe/jev-1.13"
	// OpenRouterModelJev114 adds the Box question type on OpenRouter.
	OpenRouterModelJev114 = "typesafe/jev-1.14"

	// Versioned model ids. Pin one of these rather than an alias such as
	// jev-latest, so answers and tuned thresholds do not shift on a release.
	ModelJev113 = "jev-1.13.0"
	// ModelJev114 adds the Box question type.
	ModelJev114 = "jev-1.14.0"

	maxAttempts  = 3
	maxRetryWait = 5 * time.Second
	// inputUSDPerToken is $0.042 per million input tokens; output is free.
	inputUSDPerToken = 0.042e-6
)

var (
	ErrValidation  = errors.New("typesafe: validation")
	ErrRateLimited = errors.New("typesafe: rate limited")
	errServer      = errors.New("typesafe: server error")
)

// Dependencies configures a Client. Endpoint defaults to TypeSafeEndpoint and
// HTTPClient to a 10s-timeout client. Provider is sent only when non-nil.
type Dependencies struct {
	APIKey     string
	Model      string
	Endpoint   string
	Provider   *Provider
	HTTPClient *http.Client
}

// DataCollection is OpenRouter's policy on providers that may store or train
// on request data.
type DataCollection string

const (
	DataCollectionAllow DataCollection = "allow"
	DataCollectionDeny  DataCollection = "deny"
)

// Provider is OpenRouter's provider-routing preferences; zero fields are omitted.
type Provider struct {
	DataCollection DataCollection `json:"data_collection,omitempty"`
	ZDR            bool           `json:"zdr,omitempty"`
	Only           []string       `json:"only,omitempty"`
	Order          []string       `json:"order,omitempty"`
}

type Client struct {
	apiKey       string
	model        string
	endpoint     string
	provider     *Provider
	providerName string
	http         *http.Client
}

func New(deps *Dependencies) *Client {
	c := &Client{
		apiKey: deps.APIKey, model: deps.Model, endpoint: deps.Endpoint,
		provider: deps.Provider, providerName: "typesafe", http: deps.HTTPClient,
	}
	if c.endpoint == "" {
		c.endpoint = TypeSafeEndpoint
	}
	if u, err := url.Parse(c.endpoint); err == nil && u.Hostname() == "openrouter.ai" {
		c.providerName = "openrouter"
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: 10 * time.Second}
	}
	return c
}

type request struct {
	Model     string              `json:"model"`
	State     any                 `json:"state"`
	Questions map[string]Question `json:"questions"`
	Assets    map[string]asset    `json:"assets,omitempty"`
	Provider  *Provider           `json:"provider,omitempty"`
}

// SystemOne asks every question about state in one call. 429s are retried
// honoring retry-after; a 5xx (including OpenRouter's 529) is retried once.
func (c *Client) SystemOne(ctx context.Context, state any, questions map[string]Question) (Response, error) {
	return c.SystemOneImages(ctx, state, questions)
}

// SystemOneImages is SystemOne with image assets. With images it sends a
// multipart request, or JSON with base64 assets to OpenRouter, which rejects
// multipart; with images or a Box question it sets
// DangerZoneImagesHeader. Images are checked against the API limits before
// sending (ErrInvalidImage); PrepareImage brings a photo within them.
func (c *Client) SystemOneImages(ctx context.Context, state any, questions map[string]Question, images ...Image) (Response, error) {
	ctx, span := telemetry.StartLLMCall(ctx, telemetry.LLMCallInfo{
		Operation: "chat", Provider: c.providerName, RequestModel: c.model,
	})
	res, err := c.call(ctx, state, questions, images)
	span.End(telemetry.LLMResult{
		CostUSD:       res.Usage.costUSD(),
		ResponseModel: res.Model,
		InputTokens:   res.Usage.InputTokens,
		OutputTokens:  res.Usage.OutputTokens,
	}, err)
	return res, err
}

func (c *Client) call(ctx context.Context, state any, questions map[string]Question, images []Image) (Response, error) {
	o, err := c.encode(request{Model: c.model, State: state, Questions: questions, Provider: c.provider}, images)
	if err != nil {
		return Response{}, err
	}
	serverRetried := false
	for attempt := 1; ; attempt++ {
		res, wait, err := c.post(ctx, o)
		switch {
		case err == nil:
			return res, nil
		case errors.Is(err, ErrRateLimited) && attempt < maxAttempts:
		case errors.Is(err, errServer) && !serverRetried:
			serverRetried, wait = true, 0
		default:
			return Response{}, err
		}
		select {
		case <-ctx.Done():
			return Response{}, ctx.Err()
		case <-time.After(wait):
		}
	}
}

// outgoing is an encoded request body, reused across retries.
type outgoing struct {
	body        []byte
	contentType string
	images      bool
}

func (c *Client) encode(req request, images []Image) (outgoing, error) {
	boxed := false
	for _, q := range req.Questions {
		boxed = boxed || q.Type == QuestionBox
	}
	if len(images) > 0 {
		body, contentType, err := imageBody(req, images, c.providerName == "openrouter")
		return outgoing{body: body, contentType: contentType, images: true}, err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return outgoing{}, fmt.Errorf("typesafe: marshal: %w", err)
	}
	return outgoing{body: body, contentType: "application/json", images: boxed}, nil
}

// post performs one attempt, returning how long to wait if it is retryable.
func (c *Client) post(ctx context.Context, o outgoing) (Response, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(o.body))
	if err != nil {
		return Response{}, 0, fmt.Errorf("typesafe: request: %w", err)
	}
	req.Header.Set("Content-Type", o.contentType)
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if o.images {
		req.Header.Set(DangerZoneImagesHeader, "true")
	}

	res, err := c.http.Do(req)
	if err != nil {
		return Response{}, 0, fmt.Errorf("typesafe: call: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4<<10))
		_ = res.Body.Close()
	}()

	switch {
	case res.StatusCode == http.StatusOK:
		var out Response
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			return Response{}, 0, fmt.Errorf("typesafe: decode: %w", err)
		}
		return out, 0, nil
	case res.StatusCode == http.StatusTooManyRequests:
		return Response{}, retryAfter(res.Header.Get("retry-after")), ErrRateLimited
	case res.StatusCode == http.StatusUnprocessableEntity:
		return Response{}, 0, fmt.Errorf("%w: %s", ErrValidation, validationDetail(res.Body))
	case res.StatusCode >= 500:
		return Response{}, 0, fmt.Errorf("%w: status %d: %s", errServer, res.StatusCode, errorSnippet(res.Body))
	}
	return Response{}, 0, fmt.Errorf("typesafe: status %d: %s", res.StatusCode, errorSnippet(res.Body))
}

// errorSnippet prefers OpenRouter's {"error":{"message"}} over the raw body.
func errorSnippet(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 4<<10))
	var out struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(b, &out) == nil && out.Error.Message != "" {
		return out.Error.Message
	}
	return strings.TrimSpace(string(b[:min(len(b), 512)]))
}

func retryAfter(h string) time.Duration {
	secs, err := strconv.Atoi(h)
	if err != nil || secs < 0 {
		return time.Second
	}
	return min(time.Duration(secs)*time.Second, maxRetryWait)
}

func validationDetail(r io.Reader) string {
	var out struct {
		Detail []struct {
			Loc  []any  `json:"loc"`
			Msg  string `json:"msg"`
			Type string `json:"type"`
		} `json:"detail"`
	}
	if err := json.NewDecoder(r).Decode(&out); err != nil {
		return "unreadable detail"
	}
	parts := make([]string, 0, len(out.Detail))
	for _, d := range out.Detail {
		parts = append(parts, fmt.Sprintf("%v: %s", d.Loc, d.Msg))
	}
	return strings.Join(parts, "; ")
}
