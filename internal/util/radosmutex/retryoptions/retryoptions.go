package retryoptions

import "time"

type RetryOptions struct {
	MaxAttempts   int
	SleepDuration time.Duration
}
