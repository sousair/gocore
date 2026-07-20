// Package backfill provides the reusable batch loop and registry behind a
// project's data migrations: an idempotent, batched, rate-limited job per
// data migration (typically driven by a River worker in the consuming app).
// See the AI Jail wiki concept "backfill".
package backfill

import (
	"context"
	"time"
)

// Batch is the result of processing one batch: the cursor to resume from and
// whether the backfill has finished.
type Batch[C any] struct {
	Cursor C
	Done   bool
}

// Step processes exactly one batch starting at cursor. Implementations MUST be
// idempotent (safe to re-run a batch) and MUST commit their own batch.
type Step[C any] func(ctx context.Context, cursor C) (Batch[C], error)

// Run drives step from start, sleeping throttle between batches, until a batch
// reports Done or ctx is cancelled.
func Run[C any](ctx context.Context, start C, throttle time.Duration, step Step[C]) error {
	cursor := start
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		b, err := step(ctx, cursor)
		if err != nil {
			return err
		}
		if b.Done {
			return nil
		}
		cursor = b.Cursor
		if throttle > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(throttle):
			}
		}
	}
}

// Descriptor is the catalog entry for one backfill, surfaced by `ops backfill list`.
type Descriptor struct {
	Name    string
	Summary string
}

// Registry is the per-app catalog of available backfills.
type Registry struct {
	items  []Descriptor
	byName map[string]Descriptor
}

// Register adds d to the registry.
func (r *Registry) Register(d Descriptor) {
	if r.byName == nil {
		r.byName = map[string]Descriptor{}
	}
	r.items = append(r.items, d)
	r.byName[d.Name] = d
}

// List returns all registered descriptors in registration order.
func (r *Registry) List() []Descriptor { return r.items }

// Get looks up a descriptor by name.
func (r *Registry) Get(name string) (Descriptor, bool) {
	d, ok := r.byName[name]
	return d, ok
}
