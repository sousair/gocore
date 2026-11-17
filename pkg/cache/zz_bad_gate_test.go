package cache

import "testing"

func TestBadGateIgnoredInTests(t *testing.T) {
	s := &badService{deps: &Dependencies{Store: nil, Name: "x"}}
	if s.deps.Store == nil {
		t.Skip("test-file guard must not trip the deps-nil gate")
	}
}
