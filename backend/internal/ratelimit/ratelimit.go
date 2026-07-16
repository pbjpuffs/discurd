// Package ratelimit implements Redis fixed-window rate limiting under keys
// `rl:{scope}:{id}:{window}` (docs/ARCHITECTURE.md §5, §7).
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter counts hits in fixed windows.
type Limiter struct {
	rdb *redis.Client
}

// NewLimiter builds a limiter.
func NewLimiter(rdb *redis.Client) *Limiter { return &Limiter{rdb: rdb} }

// Allow increments the counter for (scope, id) in the current window and
// reports whether the caller is within `limit` requests per `window`.
// It fails open on Redis errors so a Redis blip does not take the API down.
func (l *Limiter) Allow(ctx context.Context, scope, id string, limit int, window time.Duration) (bool, error) {
	windowStart := time.Now().Unix() / int64(window.Seconds())
	key := fmt.Sprintf("rl:%s:%s:%d", scope, id, windowStart)

	pipe := l.rdb.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return true, err
	}
	return incr.Val() <= int64(limit), nil
}
