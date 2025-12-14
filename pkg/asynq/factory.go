package asynq

import (
	"context"
	"errors"

	asynqp "github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/sousair/gocore/pkg/event"
)

type asynq struct {
	client    *asynqp.Client
	muxServer *asynqp.ServeMux
}

var _ event.Emitter = (*asynq)(nil)
var _ event.Listener = (*asynq)(nil)

func New(redisClient redis.UniversalClient) *asynq {
	client := asynqp.NewClientFromRedisClient(redisClient)
	mux := asynqp.NewServeMux()

	return &asynq{
		client:    client,
		muxServer: mux,
	}
}

func (a *asynq) Shutdown(ctx context.Context) error {
	if err := a.client.Close(); err != nil {
		return errors.Join(errors.New("[Asynq] failed to close client"), err)
	}

	return nil
}
