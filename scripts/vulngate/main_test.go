package main

import (
	"strings"
	"testing"
	"time"
)

// A govulncheck -format json stream: a config message, one symbol-level
// finding (trace[0] names a function) and one module-level finding (it does
// not), which text mode only lists as informational.
const stream = `{"config":{"scanner_name":"govulncheck"}}
{"finding":{"osv":"GO-2026-1","trace":[{"module":"m","function":"F"}]}}
{"finding":{"osv":"GO-2026-2","trace":[{"module":"m"}]}}
`

var today = time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC)

func TestGate(t *testing.T) {
	t.Run("given a listed, unexpired called finding, it passes", func(t *testing.T) {
		if err := gate(strings.NewReader(stream), "GO-2026-1 2026-11-10 reason here", today); err != nil {
			t.Fatalf("want pass, got %v", err)
		}
	})
	t.Run("given an unlisted called finding, it fails naming the ID", func(t *testing.T) {
		err := gate(strings.NewReader(stream), "# nothing listed", today)
		if err == nil || !strings.Contains(err.Error(), "GO-2026-1") {
			t.Fatalf("want failure naming GO-2026-1, got %v", err)
		}
	})
	t.Run("given a module-level finding only, it ignores it", func(t *testing.T) {
		in := `{"config":{}}` + "\n" + `{"finding":{"osv":"GO-2026-2","trace":[{"module":"m"}]}}`
		if err := gate(strings.NewReader(in), "", today); err != nil {
			t.Fatalf("want pass, got %v", err)
		}
	})
	t.Run("given an expired entry, it fails naming the entry", func(t *testing.T) {
		err := gate(strings.NewReader(stream), "GO-2026-1 2026-11-09 reason here", today)
		if err == nil || !strings.Contains(err.Error(), "GO-2026-1") || !strings.Contains(err.Error(), "expired") {
			t.Fatalf("want expiry failure, got %v", err)
		}
	})
	t.Run("given an expired entry with no finding, it still fails", func(t *testing.T) {
		in := `{"config":{}}`
		if err := gate(strings.NewReader(in), "GO-2026-9 2026-01-01 stale", today); err == nil {
			t.Fatal("want expiry failure, got pass")
		}
	})
	t.Run("given input without a config message, it fails closed", func(t *testing.T) {
		if err := gate(strings.NewReader(""), "", today); err == nil {
			t.Fatal("want failure on empty input, got pass")
		}
	})
	t.Run("given malformed JSON, it fails closed", func(t *testing.T) {
		if err := gate(strings.NewReader(`{"config":{}} {"finding":`), "", today); err == nil {
			t.Fatal("want failure on truncated input, got pass")
		}
	})
	for name, line := range map[string]string{
		"missing reason": "GO-2026-1 2026-12-01",
		"bad date":       "GO-2026-1 2026-13-01 reason",
		"bad id":         "CVE-1 2026-12-01 reason",
	} {
		t.Run("given an allowlist line with "+name+", it fails", func(t *testing.T) {
			if err := gate(strings.NewReader(stream), line, today); err == nil {
				t.Fatalf("want failure for %q, got pass", line)
			}
		})
	}
}
