// Package ws implements the gateway: WebSocket sessions, the NATS-fed hub,
// and event dispatch per docs/ARCHITECTURE.md §8.
package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"

	"github.com/gocql/gocql"
	"github.com/nats-io/nats.go"

	"discurd/internal/auth"
	"discurd/internal/events"
	"discurd/internal/obs"
	"discurd/internal/presence"
	"discurd/internal/store"
)

// Hub owns the session registry and routes NATS events to sessions.
type Hub struct {
	logger   *slog.Logger
	metrics  *obs.Metrics
	jwt      *auth.JWT
	users    *store.Users
	guilds   *store.Guilds
	presence *presence.Tracker
	pub      *events.Publisher

	mu       sync.RWMutex
	sessions map[*Session]struct{}
	closing  bool
}

// NewHub builds the hub.
func NewHub(logger *slog.Logger, metrics *obs.Metrics, jwt *auth.JWT,
	users *store.Users, guilds *store.Guilds, tracker *presence.Tracker,
	pub *events.Publisher) *Hub {
	return &Hub{
		logger:   logger,
		metrics:  metrics,
		jwt:      jwt,
		users:    users,
		guilds:   guilds,
		presence: tracker,
		pub:      pub,
		sessions: make(map[*Session]struct{}),
	}
}

// Subscribe attaches the hub to `discurd.events.>`.
func (h *Hub) Subscribe(nc *nats.Conn) (*nats.Subscription, error) {
	return nc.Subscribe(events.SubjectWildcard, h.handleNATS)
}

func (h *Hub) register(s *Session) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closing {
		return false
	}
	h.sessions[s] = struct{}{}
	return true
}

func (h *Hub) unregister(s *Session) {
	h.mu.Lock()
	delete(h.sessions, s)
	h.mu.Unlock()
}

// handleNATS routes one event envelope to matching sessions.
func (h *Hub) handleNATS(msg *nats.Msg) {
	var env events.Envelope
	if err := json.Unmarshal(msg.Data, &env); err != nil {
		h.logger.Warn("dropping malformed event", "subject", msg.Subject, "error", err.Error())
		return
	}

	frame, err := json.Marshal(serverFrame{Op: opDispatch, T: env.T, D: env.D})
	if err != nil {
		h.logger.Error("marshal dispatch frame", "error", err.Error())
		return
	}

	switch {
	case strings.HasPrefix(msg.Subject, "discurd.events.guild."):
		guildID := strings.TrimPrefix(msg.Subject, "discurd.events.guild.")
		h.dispatchToGuild(guildID, env.T, frame)
	case strings.HasPrefix(msg.Subject, "discurd.events.user."):
		userID := strings.TrimPrefix(msg.Subject, "discurd.events.user.")
		h.dispatchToUser(userID, env.T, env.GuildID, frame)
	default:
		h.logger.Warn("event on unexpected subject", "subject", msg.Subject)
	}
}

func (h *Hub) dispatchToGuild(guildID, eventType string, frame []byte) {
	gid, err := gocql.ParseUUID(guildID)
	if err != nil {
		return
	}
	h.mu.RLock()
	targets := make([]*Session, 0, len(h.sessions))
	for s := range h.sessions {
		if s.inGuild(gid) {
			targets = append(targets, s)
		}
	}
	h.mu.RUnlock()

	for _, s := range targets {
		if s.send(frame) {
			h.metrics.WSDispatched.WithLabelValues(eventType).Inc()
		}
	}
}

func (h *Hub) dispatchToUser(userID, eventType, guildID string, frame []byte) {
	h.mu.RLock()
	targets := make([]*Session, 0, 2)
	for s := range h.sessions {
		if s.userIDString() == userID {
			targets = append(targets, s)
		}
	}
	h.mu.RUnlock()

	// A GUILD_CREATE means the user's guild set changed. Add the new guild id
	// to each session's set directly from the envelope — never query Scylla
	// here: handleNATS runs on the single async-subscription goroutine, so any
	// blocking I/O would stall dispatch of every other event gateway-wide.
	var newGuild *gocql.UUID
	if eventType == events.TypeGuildCreate {
		if gid, err := gocql.ParseUUID(guildID); err == nil {
			newGuild = &gid
		}
	}
	for _, s := range targets {
		if newGuild != nil {
			s.addGuild(*newGuild)
		}
		if s.send(frame) {
			h.metrics.WSDispatched.WithLabelValues(eventType).Inc()
		}
	}
}

// broadcastPresence publishes PRESENCE_UPDATE for the user to every guild
// they belong to.
func (h *Hub) broadcastPresence(ctx context.Context, userID gocql.UUID, status string) {
	guildIDs, err := h.guilds.UserGuildIDs(ctx, userID)
	if err != nil {
		h.logger.Error("load user guilds for presence", "user_id", userID.String(), "error", err.Error())
		return
	}
	for _, gid := range guildIDs {
		payload := map[string]string{
			"user_id":  userID.String(),
			"guild_id": gid.String(),
			"status":   status,
		}
		if err := h.pub.ToGuild(gid.String(), events.TypePresenceUpdate, payload); err != nil {
			h.logger.Error("publish presence update", "error", err.Error())
		}
	}
}

// DropAllSessions closes every current session (without refusing new ones) so
// clients reconnect, re-identify, and resync. Used after a NATS reconnect:
// core NATS is at-most-once, so events published while the subscription was
// down are lost, and a forced resync is the simplest way to close that gap.
func (h *Hub) DropAllSessions(reason string) {
	h.mu.RLock()
	targets := make([]*Session, 0, len(h.sessions))
	for s := range h.sessions {
		targets = append(targets, s)
	}
	h.mu.RUnlock()
	for _, s := range targets {
		s.close(closeGoingAway, reason)
	}
}

// Shutdown closes every session gracefully and stops accepting new ones.
func (h *Hub) Shutdown() {
	h.mu.Lock()
	h.closing = true
	targets := make([]*Session, 0, len(h.sessions))
	for s := range h.sessions {
		targets = append(targets, s)
	}
	h.mu.Unlock()

	var wg sync.WaitGroup
	for _, s := range targets {
		wg.Add(1)
		go func(sess *Session) {
			defer wg.Done()
			sess.close(closeGoingAway, "server shutting down")
			<-sess.done
		}(s)
	}
	wg.Wait()
}
