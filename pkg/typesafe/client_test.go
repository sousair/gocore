package typesafe

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const okBody = `{"model":"jev-1.13.0","answers":{
 "cat":{"type":"choice","choice":"calm","confidence":0.8,"probabilities":{"calm":0.8,"wild":0.2}},
 "fits":{"type":"noul","noul":0.98}},
 "usage":{"input_tokens":120,"output_tokens":0}}`

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(&Dependencies{APIKey: "k", Model: "jev-1.13.0", BaseURL: srv.URL})
}

func questions() map[string]Question {
	return map[string]Question{
		"cat":  Choice("pick", map[string]any{"calm": "c", "wild": "w"}),
		"fits": Noul("fits?"),
	}
}

func TestSystemOne_sendsContract(t *testing.T) {
	var body map[string]any
	var method, path, auth string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		method, path, auth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(okBody))
	})

	res, err := c.SystemOne(context.Background(), map[string]any{"x": 1}, questions())
	if err != nil {
		t.Fatalf("SystemOne: %v", err)
	}
	qs := body["questions"].(map[string]any)
	if method != http.MethodPost || path != "/v1/systemone" || auth != "Bearer k" {
		t.Errorf("got %s %s auth=%q", method, path, auth)
	}
	if body["model"] != "jev-1.13.0" ||
		qs["cat"].(map[string]any)["type"] != "choice" ||
		qs["fits"].(map[string]any)["type"] != "noul" {
		t.Errorf("bad body: %v", body)
	}
	if res.Answers["cat"].Choice != "calm" || res.Answers["cat"].Probabilities["calm"] != 0.8 ||
		res.Answers["fits"].Noul != 0.98 || res.Usage.InputTokens != 120 {
		t.Errorf("bad response: %+v", res)
	}
}

func TestSystemOne_422(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"detail":[{"loc":["body","state"],"msg":"Field required","type":"missing"}]}`))
	})
	_, err := c.SystemOne(context.Background(), nil, questions())
	if !errors.Is(err, ErrValidation) || !strings.Contains(err.Error(), "Field required") {
		t.Fatalf("got %v", err)
	}
}

func TestSystemOne_429RetriesThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("retry-after", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(okBody))
	})
	if _, err := c.SystemOne(context.Background(), nil, questions()); err != nil {
		t.Fatalf("SystemOne: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", calls.Load())
	}
}

func TestSystemOne_429Exhausted(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("retry-after", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	_, err := c.SystemOne(context.Background(), nil, questions())
	if !errors.Is(err, ErrRateLimited) || calls.Load() != 3 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestSystemOne_5xxRetriedOnce(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	})
	if _, err := c.SystemOne(context.Background(), nil, questions()); err == nil || calls.Load() != 2 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestSystemOne_contextCancel(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("retry-after", "5")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.SystemOne(ctx, nil, questions())
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatalf("err=%v after %s", err, time.Since(start))
	}
}

func TestAnswerRanked(t *testing.T) {
	got := Answer{Probabilities: map[string]float64{"a": .2, "b": .5, "c": .3}}.Ranked()
	if got[0].Option != "b" || got[1].Option != "c" || got[2].Option != "a" {
		t.Errorf("got %v", got)
	}
}

func TestSystemOne_errorBodySurfaced(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("invalid api key"))
	})
	_, err := c.SystemOne(context.Background(), nil, questions())
	if err == nil || !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("got %v", err)
	}
}
