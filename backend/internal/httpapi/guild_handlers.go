package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gocql/gocql"

	"discurd/internal/events"
	"discurd/internal/models"
	"discurd/internal/store"
)

func parseUUIDParam(w http.ResponseWriter, r *http.Request, name string) (gocql.UUID, bool) {
	id, err := gocql.ParseUUID(chi.URLParam(r, name))
	if err != nil {
		writeError(w, http.StatusNotFound, CodeNotFound, name+" not found")
		return gocql.UUID{}, false
	}
	return id, true
}

func (s *Server) publishEvent(publish func() error, eventType string) {
	if err := publish(); err != nil {
		s.logger.Error("publish event failed", "type", eventType, "error", err.Error())
	}
}

func (s *Server) handleCreateGuild(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "not authenticated")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if err := ValidateGuildName(req.Name); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, err.Error())
		return
	}

	now := time.Now().UTC()
	guildID, err := gocql.RandomUUID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "internal server error")
		return
	}
	guild := store.GuildRecord{ID: guildID, Name: req.Name, OwnerID: uid, CreatedAt: now}
	if err := s.guilds.Create(r.Context(), guild); err != nil {
		writeStoreError(w, s.logger, err, "guild not found")
		return
	}
	if err := s.guilds.AddMember(r.Context(), guildID, uid, now); err != nil {
		writeStoreError(w, s.logger, err, "guild not found")
		return
	}

	// Auto-create #general.
	chanID, err := gocql.RandomUUID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "internal server error")
		return
	}
	general := store.ChannelRecord{ID: chanID, GuildID: guildID, Name: "general", Type: "text", CreatedAt: now}
	if err := s.channels.Create(r.Context(), general); err != nil {
		writeStoreError(w, s.logger, err, "guild not found")
		return
	}

	model := guild.Model()
	s.publishEvent(func() error {
		return s.publisher.ToUser(uid.String(), events.TypeGuildCreate, guildID.String(), model)
	}, events.TypeGuildCreate)

	writeJSON(w, http.StatusCreated, model)
}

func (s *Server) handleGetGuild(w http.ResponseWriter, r *http.Request) {
	uid, _ := userIDFrom(r.Context())
	guildID, ok := parseUUIDParam(w, r, "guild_id")
	if !ok {
		return
	}
	guild, ok := s.requireMember(w, r, guildID, uid)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, guild.Model())
}

func (s *Server) handleListChannels(w http.ResponseWriter, r *http.Request) {
	uid, _ := userIDFrom(r.Context())
	guildID, ok := parseUUIDParam(w, r, "guild_id")
	if !ok {
		return
	}
	if _, ok := s.requireMember(w, r, guildID, uid); !ok {
		return
	}
	recs, err := s.channels.ListByGuild(r.Context(), guildID)
	if err != nil {
		writeStoreError(w, s.logger, err, "guild not found")
		return
	}
	out := make([]models.Channel, 0, len(recs))
	for _, c := range recs {
		out = append(out, c.Model())
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	uid, _ := userIDFrom(r.Context())
	guildID, ok := parseUUIDParam(w, r, "guild_id")
	if !ok {
		return
	}
	guild, err := s.guilds.Get(r.Context(), guildID)
	if err != nil {
		writeStoreError(w, s.logger, err, "guild not found")
		return
	}
	if guild.OwnerID != uid {
		writeError(w, http.StatusForbidden, CodeForbidden, "only the guild owner can create channels")
		return
	}

	var req struct {
		Name  string `json:"name"`
		Type  string `json:"type"`
		Topic string `json:"topic"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	name, err := SanitizeChannelName(req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, err.Error())
		return
	}
	chanType, err := ValidateChannelType(req.Type)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, err.Error())
		return
	}
	if err := ValidateTopic(req.Topic); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, err.Error())
		return
	}

	chanID, err := gocql.RandomUUID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "internal server error")
		return
	}
	rec := store.ChannelRecord{
		ID:        chanID,
		GuildID:   guildID,
		Name:      name,
		Type:      chanType,
		Topic:     strings.TrimSpace(req.Topic),
		CreatedAt: time.Now().UTC(),
	}
	if err := s.channels.Create(r.Context(), rec); err != nil {
		writeStoreError(w, s.logger, err, "guild not found")
		return
	}

	model := rec.Model()
	s.publishEvent(func() error {
		return s.publisher.ToGuild(guildID.String(), events.TypeChannelCreate, model)
	}, events.TypeChannelCreate)

	writeJSON(w, http.StatusCreated, model)
}

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	uid, _ := userIDFrom(r.Context())
	guildID, ok := parseUUIDParam(w, r, "guild_id")
	if !ok {
		return
	}
	guild, ok := s.requireMember(w, r, guildID, uid)
	if !ok {
		return
	}

	rows, err := s.guilds.Members(r.Context(), guildID)
	if err != nil {
		writeStoreError(w, s.logger, err, "guild not found")
		return
	}

	ids := make([]gocql.UUID, len(rows))
	strIDs := make([]string, len(rows))
	for i, m := range rows {
		ids[i] = m.UserID
		strIDs[i] = m.UserID.String()
	}
	usersByID, err := s.userCache.GetByIDs(r.Context(), ids)
	if err != nil {
		writeStoreError(w, s.logger, err, "guild not found")
		return
	}
	statuses, err := s.presence.Statuses(r.Context(), strIDs)
	if err != nil {
		s.logger.Warn("presence lookup failed, defaulting to offline", "error", err.Error())
		statuses = map[string]string{}
	}

	out := make([]models.Member, 0, len(rows))
	for _, m := range rows {
		u := usersByID[m.UserID]
		status := statuses[m.UserID.String()]
		if status == "" {
			status = "offline"
		}
		out = append(out, models.Member{
			UserID:    m.UserID.String(),
			Username:  u.Username,
			AvatarURL: u.AvatarURL,
			JoinedAt:  m.JoinedAt.UTC(),
			Status:    status,
			IsOwner:   m.UserID == guild.OwnerID,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	uid, _ := userIDFrom(r.Context())
	guildID, ok := parseUUIDParam(w, r, "guild_id")
	if !ok {
		return
	}
	if _, ok := s.requireMember(w, r, guildID, uid); !ok {
		return
	}

	// Retry a couple of times on the astronomically unlikely code collision.
	for attempt := 0; attempt < 3; attempt++ {
		code, err := store.NewInviteCode()
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "internal server error")
			return
		}
		rec := store.InviteRecord{Code: code, GuildID: guildID, InviterID: uid, CreatedAt: time.Now().UTC()}
		err = s.invites.Create(r.Context(), rec)
		if err == nil {
			writeJSON(w, http.StatusCreated, rec.Model())
			return
		}
		if !errors.Is(err, store.ErrConflict) {
			writeStoreError(w, s.logger, err, "guild not found")
			return
		}
	}
	writeError(w, http.StatusInternalServerError, CodeInternal, "could not allocate invite code")
}

func (s *Server) handleJoinInvite(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "not authenticated")
		return
	}
	code := chi.URLParam(r, "code")

	invite, err := s.invites.Get(r.Context(), code)
	if err != nil {
		writeStoreError(w, s.logger, err, "invite not found")
		return
	}
	guild, err := s.guilds.Get(r.Context(), invite.GuildID)
	if err != nil {
		writeStoreError(w, s.logger, err, "guild not found")
		return
	}

	already, err := s.guilds.IsMember(r.Context(), invite.GuildID, uid)
	if err != nil {
		writeStoreError(w, s.logger, err, "guild not found")
		return
	}
	if already { // idempotent join
		writeJSON(w, http.StatusOK, guild.Model())
		return
	}

	now := time.Now().UTC()
	if err := s.guilds.AddMember(r.Context(), invite.GuildID, uid, now); err != nil {
		writeStoreError(w, s.logger, err, "guild not found")
		return
	}

	user, err := s.users.GetByID(r.Context(), uid)
	if err != nil {
		writeStoreError(w, s.logger, err, "user not found")
		return
	}

	status := "offline"
	if statuses, perr := s.presence.Statuses(r.Context(), []string{uid.String()}); perr == nil {
		status = statuses[uid.String()]
	}

	guildIDStr := invite.GuildID.String()
	member := models.Member{
		UserID:    uid.String(),
		Username:  user.Username,
		AvatarURL: user.AvatarURL,
		JoinedAt:  now,
		Status:    status,
		IsOwner:   false,
	}
	s.publishEvent(func() error {
		// The event payload carries guild_id on top of the REST Member shape so
		// clients can route the event without a member-list refetch (§6).
		return s.publisher.ToGuild(guildIDStr, events.TypeGuildMemberAdd, struct {
			models.Member
			GuildID string `json:"guild_id"`
		}{member, guildIDStr})
	}, events.TypeGuildMemberAdd)
	s.publishEvent(func() error {
		return s.publisher.ToUser(uid.String(), events.TypeGuildCreate, guildIDStr, guild.Model())
	}, events.TypeGuildCreate)

	writeJSON(w, http.StatusOK, guild.Model())
}
