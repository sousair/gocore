package event

import "time"

func WithMaxRetries(maxRetries int) Option {
	return func(eo *EventOptions) {
		eo.MaxRetries = maxRetries
	}
}

func WithDelay(delay time.Duration) Option {
	return func(eo *EventOptions) {
		eo.Delay = delay
	}
}

func WithProcessAt(processAt time.Time) Option {
	return func(eo *EventOptions) {
		eo.ProcessAt = processAt
	}
}
