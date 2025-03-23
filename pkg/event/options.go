package event

import "time"

// Option

func WithPrefixEmitHandler(prefix string, handler EmitHandler) Option {
	return func(e *eventEmitter) {
		e.prefixes = append(e.prefixes, prefix)
		e.handlers[EventType(prefix)] = handler
	}
}

func WithEmitHandler(eventType EventType, handler EmitHandler) Option {
	return func(e *eventEmitter) {
		e.handlers[eventType] = handler
	}
}

// EmitOption

func WithMaxRetries(maxRetries int) EmitOption {
	return func(eo *EventOption) {
		eo.MaxRetries = maxRetries
	}
}

func WithDelay(delay time.Duration) EmitOption {
	return func(eo *EventOption) {
		eo.Delay = delay
	}
}

func WithProcessAt(processAt time.Time) EmitOption {
	return func(eo *EventOption) {
		eo.ProcessAt = processAt
	}
}
