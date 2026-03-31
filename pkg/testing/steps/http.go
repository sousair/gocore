//go:build e2e

package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"
)

type HTTPResponse struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

type HTTPSteps struct {
	world     *World
	client    *http.Client
	baseURL   string
	headers   map[string]string
	Responses []HTTPResponse
}

func NewHTTPSteps(baseURL string, world *World) *HTTPSteps {
	return &HTTPSteps{
		world:   world,
		client:  &http.Client{Timeout: 120 * time.Second},
		baseURL: baseURL,
		headers: make(map[string]string),
	}
}

func (h *HTTPSteps) Reset() {
	h.headers = make(map[string]string)
	h.Responses = nil
}

func (h *HTTPSteps) LastResponse() *HTTPResponse {
	if len(h.Responses) == 0 {
		return nil
	}
	return &h.Responses[len(h.Responses)-1]
}

func (h *HTTPSteps) LastBody() string {
	if r := h.LastResponse(); r != nil {
		return string(r.Body)
	}
	return ""
}

func (h *HTTPSteps) ResponseField(field string) string {
	var data map[string]interface{}
	if err := json.Unmarshal(h.LastResponse().Body, &data); err != nil {
		return ""
	}
	if val, ok := data[field]; ok {
		return fmt.Sprintf("%v", val)
	}
	return ""
}

func (h *HTTPSteps) Post(path string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	return h.execute(context.Background(), http.MethodPost, path, bytes.NewReader(data), "application/json")
}

func (h *HTTPSteps) Get(path string) error {
	return h.execute(context.Background(), http.MethodGet, path, nil, "")
}

func (h *HTTPSteps) doRequest(ctx context.Context, method, path string, body io.Reader, contentType string) (HTTPResponse, error) {
	url := h.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return HTTPResponse{}, fmt.Errorf("create request: %w", err)
	}

	for k, v := range h.headers {
		req.Header.Set(k, v)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return HTTPResponse{}, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return HTTPResponse{}, fmt.Errorf("read body: %w", err)
	}

	return HTTPResponse{
		StatusCode: resp.StatusCode,
		Body:       data,
		Headers:    resp.Header,
	}, nil
}

func (h *HTTPSteps) execute(ctx context.Context, method, path string, body io.Reader, contentType string) error {
	resp, err := h.doRequest(ctx, method, path, body, contentType)
	if err != nil {
		return err
	}

	h.Responses = append(h.Responses, resp)

	if h.world != nil {
		h.world.SetResponseData(resp.Body)
	}

	return nil
}

func (h *HTTPSteps) executeConcurrent(ctx context.Context, count int, method, path string, body []byte, contentType string) error {
	responses := make([]HTTPResponse, count)
	errs := make([]error, count)

	var wg sync.WaitGroup
	wg.Add(count)

	for i := range count {
		go func(idx int) {
			defer wg.Done()
			var reader io.Reader
			if len(body) > 0 {
				reader = bytes.NewReader(body)
			}
			resp, err := h.doRequest(ctx, method, path, reader, contentType)
			if err != nil {
				errs[idx] = err
				return
			}
			responses[idx] = resp
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("request %d: %w", i, err)
		}
	}

	h.Responses = responses
	if len(responses) > 0 && h.world != nil {
		h.world.SetResponseData(responses[len(responses)-1].Body)
	}

	return nil
}

func (h *HTTPSteps) iSetBaseURL(_ context.Context, url string) {
	h.baseURL = url
}

func (h *HTTPSteps) iSetHeader(_ context.Context, name, value string) {
	h.headers[name] = value
}

func (h *HTTPSteps) iSetHeaders(_ context.Context, table *godog.Table) {
	for _, row := range table.Rows[1:] {
		if len(row.Cells) >= 2 {
			h.headers[row.Cells[0].Value] = row.Cells[1].Value
		}
	}
}

func (h *HTTPSteps) iSendRequest(ctx context.Context, method, path string) error {
	return h.execute(ctx, method, path, nil, "")
}

func (h *HTTPSteps) iSendRequestWithJSON(ctx context.Context, method, path string, body *godog.DocString) error {
	return h.execute(ctx, method, path, strings.NewReader(body.Content), "application/json")
}

func (h *HTTPSteps) iSendConcurrentRequests(ctx context.Context, count int, method, path string) error {
	return h.executeConcurrent(ctx, count, method, path, nil, "")
}

func (h *HTTPSteps) iSendConcurrentRequestsWithJSON(ctx context.Context, count int, method, path string, body *godog.DocString) error {
	var content []byte
	if body != nil {
		content = []byte(body.Content)
	}
	return h.executeConcurrent(ctx, count, method, path, content, "application/json")
}

func (h *HTTPSteps) theResponseStatusShouldBe(_ context.Context, expected int) error {
	r := h.LastResponse()
	if r == nil {
		return fmt.Errorf("no response recorded")
	}
	if r.StatusCode != expected {
		return fmt.Errorf("expected status %d, got %d. Body: %s", expected, r.StatusCode, string(r.Body))
	}
	return nil
}

func (h *HTTPSteps) allResponsesStatusShouldBe(_ context.Context, expected int) error {
	if len(h.Responses) == 0 {
		return fmt.Errorf("no responses recorded")
	}
	for i, r := range h.Responses {
		if r.StatusCode != expected {
			return fmt.Errorf("response %d: expected %d, got %d", i, expected, r.StatusCode)
		}
	}
	return nil
}

func (h *HTTPSteps) theResponseShouldContain(_ context.Context, expected string) error {
	if !strings.Contains(h.LastBody(), expected) {
		return fmt.Errorf("expected response to contain %q, got: %s", expected, h.LastBody())
	}
	return nil
}

func (h *HTTPSteps) theResponseShouldNotContain(_ context.Context, unexpected string) error {
	if strings.Contains(h.LastBody(), unexpected) {
		return fmt.Errorf("expected response NOT to contain %q", unexpected)
	}
	return nil
}

func (h *HTTPSteps) theResponseFieldShouldBe(_ context.Context, field, expected string) error {
	actual := h.ResponseField(field)
	if actual != expected {
		return fmt.Errorf("expected field %q = %q, got %q", field, expected, actual)
	}
	return nil
}

func (h *HTTPSteps) theResponseHeaderShouldBe(_ context.Context, header, expected string) error {
	r := h.LastResponse()
	if r == nil {
		return fmt.Errorf("no response recorded")
	}
	actual := r.Headers.Get(header)
	if actual != expected {
		return fmt.Errorf("expected header %q = %q, got %q", header, expected, actual)
	}
	return nil
}

func (h *HTTPSteps) theResponseHeaderShouldExist(_ context.Context, header string) error {
	r := h.LastResponse()
	if r == nil {
		return fmt.Errorf("no response recorded")
	}
	if r.Headers.Get(header) == "" {
		return fmt.Errorf("expected header %q to exist", header)
	}
	return nil
}

func (h *HTTPSteps) RegisterSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		h.Reset()
		return ctx, nil
	})

	ctx.Step(`^(?:I )?set base URL to "([^"]*)"$`, h.iSetBaseURL)
	ctx.Step(`^(?:I )?set header "([^"]*)" with value "([^"]*)"$`, h.iSetHeader)
	ctx.Step(`^(?:I )?set headers:$`, h.iSetHeaders)

	ctx.Step(`^(?:I )?send a (DELETE|GET|POST|PUT|PATCH) request to "([^"]*)"$`, h.iSendRequest)
	ctx.Step(`^(?:I )?send a (DELETE|GET|POST|PUT|PATCH) request to "([^"]*)" with the following JSON body:$`, h.iSendRequestWithJSON)
	ctx.Step(`^(?:I )?send (\d+) concurrent (DELETE|GET|POST|PUT|PATCH) requests to "([^"]*)"$`, h.iSendConcurrentRequests)
	ctx.Step(`^(?:I )?send (\d+) concurrent (DELETE|GET|POST|PUT|PATCH) requests to "([^"]*)" with the following JSON body:$`, h.iSendConcurrentRequestsWithJSON)

	ctx.Step(`^(?:the )?response (?:code|status) should be (\d+)$`, h.theResponseStatusShouldBe)
	ctx.Step(`^all responses should have status code (\d+)$`, h.allResponsesStatusShouldBe)
	ctx.Step(`^(?:the )?response (?:body )?should contain "([^"]*)"$`, h.theResponseShouldContain)
	ctx.Step(`^(?:the )?response (?:body )?should not contain "([^"]*)"$`, h.theResponseShouldNotContain)
	ctx.Step(`^(?:the )?response field "([^"]*)" should be "([^"]*)"$`, h.theResponseFieldShouldBe)
	ctx.Step(`^(?:the )?response header "([^"]*)" should be "([^"]*)"$`, h.theResponseHeaderShouldBe)
	ctx.Step(`^(?:the )?response header "([^"]*)" should exist$`, h.theResponseHeaderShouldExist)
}
