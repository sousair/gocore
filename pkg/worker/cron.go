package worker

import (
	"context"
	"fmt"
	"time"
)

type CronJob struct {
	Name     string
	Fn       func(ctx context.Context) error
	Interval time.Duration
}

type CronWorker struct {
	jobs []*CronJob
}

func NewCronWorker() *CronWorker {
	return &CronWorker{}
}

func (cw *CronWorker) RegisterJob(job *CronJob) {
	cw.jobs = append(cw.jobs, job)
}

func (cw CronWorker) Start(ctx context.Context) {
	for _, job := range cw.jobs {
		go func(job *CronJob) {
			for {
				select {
				case <-ctx.Done():
					fmt.Printf("[CronWorker] Stopping (%s) job \n", job.Name)

				case <-time.After(job.Interval):
					fmt.Printf("[CronWorker] Running (%s) job \n", job.Name)

					if err := job.Fn(ctx); err != nil {
						fmt.Printf("[CronWorker] Job (%s) returned error: %v \n", job.Name, err)
					}

					fmt.Printf("[CronWorker] Job (%s) finished \n", job.Name)
				}
			}
		}(job)
	}
}
