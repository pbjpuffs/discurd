package obs

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Retry calls fn until it succeeds or the budget elapses, backing off
// 1s → 2s → 4s → 8s (capped). Used for infra connections at startup so the
// services do not crash-loop while ScyllaDB/Redis/NATS/MinIO come up.
func Retry(ctx context.Context, logger *slog.Logger, name string, budget time.Duration, fn func() error) error {
	deadline := time.Now().Add(budget)
	delay := time.Second
	for attempt := 1; ; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		if time.Now().Add(delay).After(deadline) {
			return fmt.Errorf("connect %s: %w", name, err)
		}
		logger.Warn("connection attempt failed, retrying",
			"target", name, "attempt", attempt, "retry_in", delay.String(), "error", err.Error())
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		if delay *= 2; delay > 8*time.Second {
			delay = 8 * time.Second
		}
	}
}
