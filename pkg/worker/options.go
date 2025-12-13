package worker

import "time"

func WithCronInterval(interval time.Duration) CronOption {
	return func(co *cronOptions) {
		co.Interval = interval
	}
}

func WithSingleRunner() CronOption {
	return func(co *cronOptions) {
		co.SingleRunner = true
	}
}
