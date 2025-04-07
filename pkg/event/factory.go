package event

import (
	"errors"
	"strings"

	"golang.org/x/net/context"
)

type eventEmitter struct {
	prefixes []string
	handlers map[EventType]EmitHandler
}

type Option func(*eventEmitter)

var (
	HandlerNotFoundErr = errors.New("[EventEmitter] handler not found")
	HandlerReturnedErr = errors.New("[EventEmitter] handler returned error an error")
)

func New(opts ...Option) *eventEmitter {
	handlers := make(map[EventType]EmitHandler)

	eventEmitter := &eventEmitter{
		handlers: handlers,
	}

	for _, opt := range opts {
		opt(eventEmitter)
	}

	return eventEmitter
}

func (e *eventEmitter) Emit(ctx context.Context, event *Event, opts ...EmitOption) error {
	eType := event.Type

	eventOption := &EventOption{}
	for _, opt := range opts {
		opt(eventOption)
	}

	if len(e.prefixes) > 0 {
		for _, prefix := range e.prefixes {
			if strings.HasPrefix(eType.String(), prefix) {
				handler, ok := e.handlers[EventType(prefix)]
				if !ok {
					return HandlerNotFoundErr
				}

				return handleErr(handler(ctx, event, eventOption))
			}
		}
	}

	handler, ok := e.handlers[eType]
	if !ok {
		return HandlerNotFoundErr
	}

	return handleErr(handler(ctx, event, eventOption))
}

func handleErr(err error) error {
	if err != nil {
		return errors.Join(HandlerReturnedErr, err)
	}

	return nil
}
