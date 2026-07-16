// Package presence implements Redis-backed presence per docs/ARCHITECTURE.md
// §5 and §8: a 70s "online" key refreshed on heartbeat, plus a per-session
// liveness set deciding when to broadcast online/offline transitions.
//
// Rather than a single shared INCR/DECR counter (which underflows on a
// double-disconnect, inflates permanently if a gateway dies mid-session, and
// whose TTL is refreshed by any one of the user's sessions), liveness is a
// sorted set `presence_conns:{user}` of session-id → last-seen-unix. Every
// operation first prunes members older than the liveness window, so a session
// whose gateway crashed ages out on its own without dragging others down, and
// online/offline is derived atomically (Lua) from the true live-member count.
package presence

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	presenceTTL = 70 * time.Second
	// livenessWindow is how long a session may go without a heartbeat before
	// it is considered gone. Heartbeats arrive every ~30s (§8), so 120s
	// tolerates a couple of missed beats before the 90s read-deadline close.
	livenessWindow = 120 * time.Second

	// StatusOnline / StatusOffline are the wire status strings.
	StatusOnline  = "online"
	StatusOffline = "offline"
)

func presenceKey(userID string) string { return "presence:" + userID }
func connsKey(userID string) string    { return "presence_conns:" + userID }

// connectScript: prune stale, count live-before, add/refresh this session,
// refresh TTLs, set the presence key. Returns 1 iff this session is the first
// live one (broadcast online). KEYS: conns, presence. ARGV: session, now,
// windowSecs, presenceSecs.
var connectScript = redis.NewScript(`
local cutoff = tonumber(ARGV[2]) - tonumber(ARGV[3])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', '(' .. cutoff)
local before = redis.call('ZCARD', KEYS[1])
redis.call('ZADD', KEYS[1], ARGV[2], ARGV[1])
redis.call('EXPIRE', KEYS[1], ARGV[3])
redis.call('SET', KEYS[2], 'online', 'EX', ARGV[4])
if before == 0 then return 1 else return 0 end
`)

// heartbeatScript: prune stale, refresh this session's score and both TTLs.
var heartbeatScript = redis.NewScript(`
local cutoff = tonumber(ARGV[2]) - tonumber(ARGV[3])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', '(' .. cutoff)
redis.call('ZADD', KEYS[1], ARGV[2], ARGV[1])
redis.call('EXPIRE', KEYS[1], ARGV[3])
redis.call('SET', KEYS[2], 'online', 'EX', ARGV[4])
return 1
`)

// disconnectScript: remove this session, prune stale, and if none remain drop
// both keys. Returns 1 iff no live sessions remain (broadcast offline).
// Removing a session that was never added is a harmless no-op, so an
// unbalanced disconnect can never underflow or evict a still-live user.
var disconnectScript = redis.NewScript(`
redis.call('ZREM', KEYS[1], ARGV[1])
local cutoff = tonumber(ARGV[2]) - tonumber(ARGV[3])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', '(' .. cutoff)
local remaining = redis.call('ZCARD', KEYS[1])
if remaining == 0 then
  redis.call('DEL', KEYS[1])
  redis.call('DEL', KEYS[2])
  return 1
end
return 0
`)

// Tracker manipulates presence state in Redis.
type Tracker struct {
	rdb *redis.Client
}

// NewTracker builds a tracker.
func NewTracker(rdb *redis.Client) *Tracker { return &Tracker{rdb: rdb} }

func (t *Tracker) now() string { return strconv.FormatInt(time.Now().Unix(), 10) }
func windowSecs() string       { return strconv.Itoa(int(livenessWindow / time.Second)) }
func presenceSecs() string     { return strconv.Itoa(int(presenceTTL / time.Second)) }

// Connect records a live socket for the user. first is true when this is the
// only live session, i.e. the caller should broadcast PRESENCE_UPDATE online.
func (t *Tracker) Connect(ctx context.Context, userID, sessionID string) (first bool, err error) {
	n, err := connectScript.Run(ctx, t.rdb,
		[]string{connsKey(userID), presenceKey(userID)},
		sessionID, t.now(), windowSecs(), presenceSecs(),
	).Int()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// Heartbeat refreshes the session's liveness and both TTLs.
func (t *Tracker) Heartbeat(ctx context.Context, userID, sessionID string) error {
	return heartbeatScript.Run(ctx, t.rdb,
		[]string{connsKey(userID), presenceKey(userID)},
		sessionID, t.now(), windowSecs(), presenceSecs(),
	).Err()
}

// Disconnect records a closed socket. last is true when no live sessions
// remain, i.e. the caller should broadcast PRESENCE_UPDATE offline.
func (t *Tracker) Disconnect(ctx context.Context, userID, sessionID string) (last bool, err error) {
	n, err := disconnectScript.Run(ctx, t.rdb,
		[]string{connsKey(userID), presenceKey(userID)},
		sessionID, t.now(), windowSecs(),
	).Int()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// Statuses MGETs presence for the given users and returns a map of
// user_id → "online"|"offline".
func (t *Tracker) Statuses(ctx context.Context, userIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	keys := make([]string, len(userIDs))
	for i, id := range userIDs {
		keys[i] = presenceKey(id)
	}
	vals, err := t.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	for i, v := range vals {
		if v == nil {
			out[userIDs[i]] = StatusOffline
		} else {
			out[userIDs[i]] = StatusOnline
		}
	}
	return out, nil
}
