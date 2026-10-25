// Package scheduler contains time-driven adapters that invoke use cases on a
// schedule.
package scheduler

import (
	"context"
	"log/slog"
	"time"
)

// Counter is the subset of port.UserService the reporter needs.
type Counter interface {
	Count(ctx context.Context) (int64, error)
}

// UserCountReporter logs the total number of users at a fixed interval.
type UserCountReporter struct {
	counter  Counter
	interval time.Duration
	logger   *slog.Logger
}

// NewUserCountReporter builds a reporter.
func NewUserCountReporter(counter Counter, interval time.Duration, logger *slog.Logger) *UserCountReporter {
	return &UserCountReporter{counter: counter, interval: interval, logger: logger}
}

// Run blocks until ctx is cancelled, logging the user count every interval.
// Each count gets its own timeout so a slow database cannot pile up ticks.
func (r *UserCountReporter) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("user count reporter stopped")
			return
		case <-ticker.C:
			r.report(ctx)
		}
	}
}

func (r *UserCountReporter) report(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, r.interval)
	defer cancel()

	n, err := r.counter.Count(ctx)
	if err != nil {
		r.logger.ErrorContext(ctx, "count users failed", "error", err)
		return
	}
	r.logger.InfoContext(ctx, "total users", "count", n)
}
