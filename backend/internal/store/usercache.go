package store

import (
	"context"
	"sync"
	"time"

	"github.com/gocql/gocql"
)

// UserCache is a small in-process TTL cache in front of Users.GetByIDs, used
// for message author hydration so hot authors are not re-read every request.
type UserCache struct {
	users *Users
	ttl   time.Duration

	mu      sync.Mutex
	entries map[gocql.UUID]cacheEntry
}

type cacheEntry struct {
	user    UserRecord
	expires time.Time
}

// NewUserCache builds a cache with the given entry TTL.
func NewUserCache(users *Users, ttl time.Duration) *UserCache {
	return &UserCache{
		users:   users,
		ttl:     ttl,
		entries: make(map[gocql.UUID]cacheEntry),
	}
}

// GetByIDs returns the requested users, reading misses from ScyllaDB with a
// single IN query. Unknown ids are simply absent from the result.
func (c *UserCache) GetByIDs(ctx context.Context, ids []gocql.UUID) (map[gocql.UUID]UserRecord, error) {
	out := make(map[gocql.UUID]UserRecord, len(ids))
	var misses []gocql.UUID
	now := time.Now()

	c.mu.Lock()
	seen := make(map[gocql.UUID]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if e, ok := c.entries[id]; ok && now.Before(e.expires) {
			out[id] = e.user
		} else {
			misses = append(misses, id)
		}
	}
	c.mu.Unlock()

	if len(misses) == 0 {
		return out, nil
	}
	fetched, err := c.users.GetByIDs(ctx, misses)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	expires := now.Add(c.ttl)
	for id, u := range fetched {
		c.entries[id] = cacheEntry{user: u, expires: expires}
		out[id] = u
	}
	c.mu.Unlock()
	return out, nil
}

// Invalidate drops a user (called after username/avatar updates).
func (c *UserCache) Invalidate(id gocql.UUID) {
	c.mu.Lock()
	delete(c.entries, id)
	c.mu.Unlock()
}
