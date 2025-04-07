package event

import (
	"time"

	"golang.org/x/net/context"
)

type EventOption struct {
	MaxRetries int
	Delay      time.Duration
	ProcessAt  time.Time
}

type EventType string

func (et EventType) String() string {
	return string(et)
}

type Event struct {
	Type    EventType   `json:"type"`
	Payload interface{} `json:"payload"`
}

type EmitHandler func(ctx context.Context, event *Event, opt *EventOption) error

type EmitOption func(*EventOption)

type EventEmitter interface {
	Emit(ctx context.Context, event *Event, opts ...EmitOption) error
}
