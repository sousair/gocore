package backfill_test

import (
	"context"
	"testing"
	"time"

	"github.com/sousair/gocore/pkg/backfill"
)

func TestRun_processesAllBatchesIdempotently(t *testing.T) {
	var seen []int
	step := func(ctx context.Context, cursor int) (backfill.Batch[int], error) {
		if cursor >= 3 {
			return backfill.Batch[int]{Cursor: cursor, Done: true}, nil
		}
		seen = append(seen, cursor)
		return backfill.Batch[int]{Cursor: cursor + 1}, nil
	}
	if err := backfill.Run(context.Background(), 0, 0, step); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := len(seen); got != 3 {
		t.Fatalf("processed %d batches, want 3 (%v)", got, seen)
	}
}

func TestRun_stopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	step := func(ctx context.Context, c int) (backfill.Batch[int], error) {
		cancel()
		return backfill.Batch[int]{Cursor: c + 1}, nil
	}
	if err := backfill.Run(ctx, 0, time.Millisecond, step); err == nil {
		t.Fatal("expected context error after cancel, got nil")
	}
}

func TestRegistry_listAndGet(t *testing.T) {
	var r backfill.Registry
	r.Register(backfill.Descriptor{Name: "noop", Summary: "does nothing"})
	if len(r.List()) != 1 {
		t.Fatalf("List len = %d, want 1", len(r.List()))
	}
	if _, ok := r.Get("noop"); !ok {
		t.Fatal("Get(noop) not found")
	}
	if _, ok := r.Get("missing"); ok {
		t.Fatal("Get(missing) should be false")
	}
}
