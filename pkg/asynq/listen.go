package asynq

import (
	"context"

	asynqp "github.com/hibiken/asynq"
	"github.com/sousair/gocore/pkg/event"
)

type HandlerFunc func(ctx context.Context, t *asynqp.Task) error

func (a *asynq) AddHandler(eventType event.EventType, handler HandlerFunc) {
	a.muxServer.HandleFunc(eventType.String(), handler)
}

func (a *asynq) Listen() error {
	return nil
}
