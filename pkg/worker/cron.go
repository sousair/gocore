package worker

import (
	"context"
	"fmt"
	"time"
)

const (
	CRON_DEFAULT_TIMEOUT  = time.Minute
	CRON_DEFAULT_INTERVAL = time.Second
)

type CronJob interface {
	Name() string
	Handle(context.Context) error
}

type cronOptions struct {
	Interval time.Duration
}

type CronOption func(*cronOptions)

func WithCronInterval(interval time.Duration) CronOption {
	return func(co *cronOptions) {
		co.Interval = interval
	}
}

type cronJob struct {
	Handle  func(context.Context) error
	Options *cronOptions
}

type CronWorker struct {
	jobs map[string]*cronJob
}

func NewCronWorker() *CronWorker {
	return &CronWorker{
		jobs: make(map[string]*cronJob),
	}
}

// Use WithCronInterval to set options
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

func (cw CronWorker) Start(ctx context.Context) {
	for name, job := range cw.jobs {
		go func(name string, job *cronJob) {
			for {
				jobOptions := job.Options
				select {
				case <-ctx.Done():
					fmt.Printf("[CronWorker] Stopping (%s) job \n", name)
				case <-time.After(jobOptions.Interval):
					fmt.Printf("[CronWorker] Running (%s) job \n", name)

					if err := job.Handle(ctx); err != nil {
						fmt.Printf("[CronWorker] Job (%s) returned error: %v \n", name, err)
					}

					fmt.Printf("[CronWorker] Job (%s) finished \n", name)
				}

			}
		}(name, job)
	}
}
