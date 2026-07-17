package httpapi

import (
	"net/http"
	"time"

	"discurd/internal/events"
)

// handleEffect broadcasts an ephemeral channel effect (lightning, confetti, …)
// to every member of the guild. Effects are not persisted.
func (s *Server) handleEffect(w http.ResponseWriter, r *http.Request) {
	channel, _, uid, ok := s.channelContext(w, r)
	if !ok {
		return
	}

	var req struct {
		Type string `json:"type"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := ValidateEffectType(req.Type); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, err.Error())
		return
	}

	// Per-channel effect rate limit: 5 / 10s per user. Fail open on limiter error.
	rlID := uid.String() + ":" + channel.ID.String()
	allowed, err := s.limiter.Allow(r.Context(), "effect", rlID, 5, 10*time.Second)
	if err != nil {
		s.logger.Warn("rate limiter unavailable, failing open", "error", err.Error())
	}
	if !allowed {
		writeError(w, http.StatusTooManyRequests, CodeRateLimited, "triggering effects too fast")
		return
	}

	payload := map[string]string{
		"channel_id": channel.ID.String(),
		"guild_id":   channel.GuildID.String(),
		"type":       req.Type,
		"user_id":    uid.String(),
	}
	s.publishEvent(func() error {
		return s.publisher.ToGuild(channel.GuildID.String(), events.TypeEffect, payload)
	}, events.TypeEffect)

	w.WriteHeader(http.StatusNoContent)
}
