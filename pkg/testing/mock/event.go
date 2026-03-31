//go:build unit

package mock

import (
	"context"
	"sync"

	"github.com/sousair/gocore/pkg/event"
)

type EventEmitter struct {
	mu     sync.RWMutex
	Events []*event.Event

	EmitCalls int
}

var _ event.Emitter = (*EventEmitter)(nil)

func NewEventEmitter() *EventEmitter {
	return &EventEmitter{}
}

func (e *EventEmitter) Emit(ctx context.Context, ev *event.Event, opts ...event.Option) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.Events = append(e.Events, ev)
	e.EmitCalls++
	return nil
}

func (e *EventEmitter) Shutdown(ctx context.Context) error {
	return nil
}

func (e *EventEmitter) EventsByType(t event.EventType) []*event.Event {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var result []*event.Event
	for _, ev := range e.Events {
		if ev.Type == t {
			result = append(result, ev)
		}
	}
	return result
}

func (e *EventEmitter) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.Events = nil
	e.EmitCalls = 0
}
