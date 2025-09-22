package event

import (
	"time"

	"context"
)

type EventType string

func (t EventType) String() string {
	return string(t)
}

type Event struct {
	Type    EventType `json:"type"`
	Payload any       `json:"payload"`
}

type (
	Option func(*EventOptions)

	EventOptions struct {
		MaxRetries int
		Delay      time.Duration
		ProcessAt  time.Time
		Metadata   map[string]any
	}
)

type (
	Emitter interface {
		Emit(context.Context, *Event, ...Option) error
		Shutdown(context.Context) error
	}

	Listener interface {
		Listen() error
		Shutdown(context.Context) error
	}
)
