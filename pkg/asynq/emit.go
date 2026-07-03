package asynq

import (
	"context"
	"encoding/json"
	"log/slog"

	asynqp "github.com/hibiken/asynq"
	"github.com/sousair/gocore/pkg/event"
)

func parseEventOptions(opt *event.EventOptions) []asynqp.Option {
	var opts []asynqp.Option

	if opt.MaxRetries > 0 {
		opts = append(opts, asynqp.MaxRetry(opt.MaxRetries))
	}

	if opt.Delay > 0 {
		opts = append(opts, asynqp.ProcessIn(opt.Delay))
	}

	if !opt.ProcessAt.IsZero() {
		opts = append(opts, asynqp.ProcessAt(opt.ProcessAt))
	}

	// TODO: Handle metadata

	return opts
}

func (a *asynq) Emit(ctx context.Context, e *event.Event, opts ...event.Option) error {
	data, err := json.Marshal(e.Payload)
	if err != nil {
		return err
	}

	options := &event.EventOptions{}
	for _, opt := range opts {
		opt(options)
	}

	task := asynqp.NewTask(e.Type.String(),
		data,
		parseEventOptions(options)...,
	)

	info, err := a.client.EnqueueContext(ctx, task)
	if err != nil {
		return err
	}

	slog.InfoContext(ctx, "asynq.task_enqueued",
		slog.String("id", info.ID),
		slog.String("queue", info.Queue),
	)

	return nil
}
