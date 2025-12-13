package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/sousair/gocore/pkg/cache"
)

const (
	CRON_DEFAULT_TIMEOUT  = time.Minute
	CRON_DEFAULT_INTERVAL = time.Second

	SINGLE_RUNNER_CACHE_KEY = "cw_single:%s"
)

type CronJob interface {
	Name() string
	Handle(context.Context) error
}

type cronOptions struct {
	Interval     time.Duration
	SingleRunner bool
}

type CronOption func(*cronOptions)

type cronJob struct {
	Handle  func(context.Context) error
	Options *cronOptions
}

type CronWorker struct {
	ID            uuid.UUID
	jobs          map[string]*cronJob
	singleRunners []string
	cache         cache.Cache
}

func NewCronWorker(cache cache.Cache) *CronWorker {
	return &CronWorker{
		ID:    uuid.New(),
		jobs:  make(map[string]*cronJob),
		cache: cache,
	}
}

func (cw *CronWorker) RegisterJob(job CronJob, opts ...CronOption) {
	options := &cronOptions{
		Interval: CRON_DEFAULT_INTERVAL,
	}

	for _, opt := range opts {
		opt(options)
	}

	cw.jobs[job.Name()] = &cronJob{
		Handle:  job.Handle,
		Options: options,
	}
}

func (cw *CronWorker) Start(ctx context.Context) {
	for name, job := range cw.jobs {
		jobOptions := job.Options
		if jobOptions.SingleRunner {
			ellected, err := cw.checkAndRegisterSingleRunner(ctx, name)
			if err != nil {
				slog.ErrorContext(ctx,
					"[CronWorker] Error checking single runner",
					slog.String("job", name),
					slog.Any("error", err),
				)
			}

			if !ellected {
				slog.InfoContext(ctx,
					"[CronWorker] Another instance is already running the job",
					slog.String("job", name),
				)
				continue
			}

			cw.singleRunners = append(cw.singleRunners, name)
		}

		go func(name string, job *cronJob) {
			for {
				select {
				case <-ctx.Done():
					slog.InfoContext(ctx,
						"[CronWorker] Stopping job due to context done",
						slog.String("job", name),
					)
				case <-time.After(jobOptions.Interval):
					slog.InfoContext(ctx,
						"[CronWorker] Triggering job",
						slog.String("job", name),
					)

					if err := job.Handle(ctx); err != nil {
						slog.ErrorContext(ctx,
							"[CronWorker] Job returned error",
							slog.String("job", name),
							slog.Any("error", err),
						)
					}

					slog.InfoContext(ctx,
						"[CronWorker] Job finished",
						slog.String("job", name),
					)
				}

			}
		}(name, job)
	}
}

func (cw CronWorker) checkAndRegisterSingleRunner(ctx context.Context, name string) (bool, error) {
	cacheKey := fmt.Sprintf(SINGLE_RUNNER_CACHE_KEY, name)

	value, err := cw.cache.Get(ctx, cacheKey)
	if err != nil {
		if !errors.Is(err, cache.ErrKeyNotFound) {
			return false, err
		}
	}

	if value != "" {
		slog.InfoContext(ctx,
			"[CronWorker] Single runner already registered",
			slog.String("job", name),
			slog.String("runner_id", value),
		)
		return false, nil
	}

	release, err := cw.cache.Lock(ctx, cacheKey,
		cache.WithLockRetries(2),
		cache.WithLockRetryDelay(250*time.Millisecond),
	)
	if err != nil {
		slog.ErrorContext(ctx,
			"[CronWorker] Failed to acquire lock for single runner",
			slog.String("job", name),
			slog.Any("error", err),
		)
		return false, err
	}
	defer func() {
		if err := release(ctx); err != nil {
			slog.ErrorContext(ctx,
				"[CronWorker] Failed to release lock",
				slog.String("job", name),
				slog.Any("error", err),
			)
		}
	}()

	if err := cw.cache.Set(ctx, cacheKey, cw.ID.String()); err != nil {
		slog.ErrorContext(ctx,
			"[CronWorker] Failed to register single runner",
			slog.String("job", name),
			slog.Any("error", err),
		)
		return false, err
	}

	slog.InfoContext(ctx,
		"[CronWorker] Registered as single runner",
		slog.String("job", name),
		slog.String("runner_id", cw.ID.String()),
	)
	return true, nil
}

func (cw CronWorker) Shutdown(ctx context.Context) error {
	for _, name := range cw.singleRunners {
		cacheKey := fmt.Sprintf(SINGLE_RUNNER_CACHE_KEY, name)
		err := cw.cache.Del(ctx, cacheKey)
		if err != nil {
			slog.ErrorContext(ctx,
				"[CronWorker] Failed to deregister single runner on shutdown",
				slog.String("job", name),
				slog.Any("error", err),
			)
			return err
		}
	}

	return nil
}
