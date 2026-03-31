//go:build unit

package mock

import (
	"context"
	"testing"

	"github.com/sousair/gocore/pkg/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventEmitter_Emit(t *testing.T) {
	emitter := NewEventEmitter()
	ctx := context.Background()

	err := emitter.Emit(ctx, &event.Event{Type: "task.created", Payload: "test"})
	require.NoError(t, err)

	assert.Equal(t, 1, emitter.EmitCalls)
	assert.Len(t, emitter.Events, 1)
	assert.Equal(t, event.EventType("task.created"), emitter.Events[0].Type)
}

func TestEventEmitter_EventsByType(t *testing.T) {
	emitter := NewEventEmitter()
	ctx := context.Background()

	emitter.Emit(ctx, &event.Event{Type: "task.created"})
	emitter.Emit(ctx, &event.Event{Type: "task.completed"})
	emitter.Emit(ctx, &event.Event{Type: "task.created"})

	created := emitter.EventsByType("task.created")
	assert.Len(t, created, 2)

	completed := emitter.EventsByType("task.completed")
	assert.Len(t, completed, 1)

	missing := emitter.EventsByType("task.deleted")
	assert.Empty(t, missing)
}

func TestEventEmitter_Reset(t *testing.T) {
	emitter := NewEventEmitter()
	ctx := context.Background()

	emitter.Emit(ctx, &event.Event{Type: "test"})
	emitter.Emit(ctx, &event.Event{Type: "test"})

	emitter.Reset()

	assert.Empty(t, emitter.Events)
	assert.Equal(t, 0, emitter.EmitCalls)
}

func TestEventEmitter_Shutdown(t *testing.T) {
	emitter := NewEventEmitter()
	err := emitter.Shutdown(context.Background())
	assert.NoError(t, err)
}
