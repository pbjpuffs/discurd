package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gocql/gocql"

	"discurd/internal/events"
	"discurd/internal/models"
	"discurd/internal/store"
)

const (
	defaultMessageLimit = 50
	maxMessageLimit     = 100
)

// channelContext resolves a channel and enforces guild membership.
func (s *Server) channelContext(w http.ResponseWriter, r *http.Request) (store.ChannelRecord, store.GuildRecord, gocql.UUID, bool) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "not authenticated")
		return store.ChannelRecord{}, store.GuildRecord{}, gocql.UUID{}, false
	}
	channelID, ok := parseUUIDParam(w, r, "channel_id")
	if !ok {
		return store.ChannelRecord{}, store.GuildRecord{}, gocql.UUID{}, false
	}
	channel, err := s.channels.Get(r.Context(), channelID)
	if err != nil {
		writeStoreError(w, s.logger, err, "channel not found")
		return store.ChannelRecord{}, store.GuildRecord{}, gocql.UUID{}, false
	}
	guild, ok := s.requireMember(w, r, channel.GuildID, uid)
	if !ok {
		return store.ChannelRecord{}, store.GuildRecord{}, gocql.UUID{}, false
	}
	return channel, guild, uid, true
}

// hydrateMessages converts records to API messages with authors batch-fetched
// (one IN query per distinct author max, fronted by the TTL cache).
func (s *Server) hydrateMessages(r *http.Request, channel store.ChannelRecord, recs []store.MessageRecord) ([]models.Message, error) {
	distinct := make([]gocql.UUID, 0, 8)
	seen := make(map[gocql.UUID]bool, 8)
	for _, m := range recs {
		if !seen[m.AuthorID] {
			seen[m.AuthorID] = true
			distinct = append(distinct, m.AuthorID)
		}
	}
	authors, err := s.userCache.GetByIDs(r.Context(), distinct)
	if err != nil {
		return nil, err
	}

	out := make([]models.Message, 0, len(recs))
	for _, m := range recs {
		out = append(out, messageModel(channel, m, authors[m.AuthorID]))
	}
	return out, nil
}

// maxAttachmentsPerMessage bounds how many files one message may reference.
const maxAttachmentsPerMessage = 10

// validateAttachments rejects client-supplied attachment metadata whose URL is
// not a relative path under this channel's own upload prefix. Without this a
// caller could attach `javascript:…`, an off-origin URL, or another channel's
// object and have the client render it — the message-create body is otherwise
// trusted verbatim and broadcast to every member.
func validateAttachments(channelID string, atts []models.Attachment) error {
	if len(atts) > maxAttachmentsPerMessage {
		return fmt.Errorf("at most %d attachments per message", maxAttachmentsPerMessage)
	}
	prefix := "/files/attachments/" + channelID + "/"
	for _, a := range atts {
		if !strings.HasPrefix(a.URL, prefix) || strings.Contains(a.URL, "..") {
			return fmt.Errorf("invalid attachment url")
		}
	}
	return nil
}

func messageModel(channel store.ChannelRecord, m store.MessageRecord, author store.UserRecord) models.Message {
	attachments := m.Attachments
	if attachments == nil {
		attachments = []models.Attachment{}
	}
	return models.Message{
		ID:        m.ID.String(),
		ChannelID: channel.ID.String(),
		GuildID:   channel.GuildID.String(),
		Author: models.MessageAuthor{
			ID:        m.AuthorID.String(),
			Username:  author.Username,
			AvatarURL: author.AvatarURL,
		},
		Content:     m.Content,
		Attachments: attachments,
		CreatedAt:   m.CreatedAt(),
		EditedAt:    m.EditedAt,
	}
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	channel, _, _, ok := s.channelContext(w, r)
	if !ok {
		return
	}

	limit := defaultMessageLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, CodeValidationFailed, "limit must be a positive integer")
			return
		}
		if n > maxMessageLimit {
			n = maxMessageLimit
		}
		limit = n
	}

	var before *gocql.UUID
	if raw := r.URL.Query().Get("before"); raw != "" {
		id, err := gocql.ParseUUID(raw)
		if err != nil || id.Version() != 1 {
			writeError(w, http.StatusBadRequest, CodeValidationFailed, "before must be a message timeuuid")
			return
		}
		before = &id
	}

	recs, err := s.messages.List(r.Context(), channel.ID, channel.CreatedAt, before, limit)
	if err != nil {
		writeStoreError(w, s.logger, err, "channel not found")
		return
	}
	out, err := s.hydrateMessages(r, channel, recs)
	if err != nil {
		writeStoreError(w, s.logger, err, "channel not found")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateMessage(w http.ResponseWriter, r *http.Request) {
	channel, _, uid, ok := s.channelContext(w, r)
	if !ok {
		return
	}

	// Per-channel message rate limit: 10 / 10s per user.
	rlID := uid.String() + ":" + channel.ID.String()
	allowed, err := s.limiter.Allow(r.Context(), "msg", rlID, 10, 10*time.Second)
	if err != nil {
		s.logger.Warn("rate limiter unavailable, failing open", "error", err.Error())
	}
	if !allowed {
		writeError(w, http.StatusTooManyRequests, CodeRateLimited, "sending messages too fast")
		return
	}

	var req struct {
		Content     string              `json:"content"`
		Attachments []models.Attachment `json:"attachments"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := ValidateMessageContent(req.Content, len(req.Attachments) > 0); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, err.Error())
		return
	}
	if err := validateAttachments(channel.ID.String(), req.Attachments); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, err.Error())
		return
	}

	id := gocql.TimeUUID()
	rec := store.MessageRecord{
		ChannelID:   channel.ID,
		Bucket:      store.BucketFromTime(id.Time()),
		ID:          id,
		AuthorID:    uid,
		Content:     req.Content,
		Attachments: req.Attachments,
	}
	if err := s.messages.Insert(r.Context(), rec); err != nil {
		writeStoreError(w, s.logger, err, "channel not found")
		return
	}
	s.metrics.MessagesCreated.Inc()

	author, err := s.users.GetByID(r.Context(), uid)
	if err != nil {
		writeStoreError(w, s.logger, err, "user not found")
		return
	}
	model := messageModel(channel, rec, author)
	s.publishEvent(func() error {
		return s.publisher.ToGuild(channel.GuildID.String(), events.TypeMessageCreate, model)
	}, events.TypeMessageCreate)

	writeJSON(w, http.StatusCreated, model)
}

// loadMessage parses {message_id} and fetches the row.
func (s *Server) loadMessage(w http.ResponseWriter, r *http.Request, channel store.ChannelRecord) (store.MessageRecord, bool) {
	raw := chi.URLParam(r, "message_id")
	id, err := gocql.ParseUUID(raw)
	if err != nil || id.Version() != 1 {
		writeError(w, http.StatusNotFound, CodeNotFound, "message not found")
		return store.MessageRecord{}, false
	}
	rec, err := s.messages.Get(r.Context(), channel.ID, id)
	if err != nil {
		writeStoreError(w, s.logger, err, "message not found")
		return store.MessageRecord{}, false
	}
	return rec, true
}

func (s *Server) handleEditMessage(w http.ResponseWriter, r *http.Request) {
	channel, _, uid, ok := s.channelContext(w, r)
	if !ok {
		return
	}
	rec, ok := s.loadMessage(w, r, channel)
	if !ok {
		return
	}
	if rec.AuthorID != uid {
		writeError(w, http.StatusForbidden, CodeForbidden, "you can only edit your own messages")
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := ValidateMessageContent(req.Content, len(rec.Attachments) > 0); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, err.Error())
		return
	}

	editedAt := time.Now().UTC()
	if err := s.messages.UpdateContent(r.Context(), rec, req.Content, editedAt); err != nil {
		writeStoreError(w, s.logger, err, "message not found")
		return
	}
	rec.Content = req.Content
	rec.EditedAt = &editedAt

	author, err := s.users.GetByID(r.Context(), uid)
	if err != nil {
		writeStoreError(w, s.logger, err, "user not found")
		return
	}
	model := messageModel(channel, rec, author)
	s.publishEvent(func() error {
		return s.publisher.ToGuild(channel.GuildID.String(), events.TypeMessageUpdate, model)
	}, events.TypeMessageUpdate)

	writeJSON(w, http.StatusOK, model)
}

func (s *Server) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	channel, guild, uid, ok := s.channelContext(w, r)
	if !ok {
		return
	}
	rec, ok := s.loadMessage(w, r, channel)
	if !ok {
		return
	}
	if rec.AuthorID != uid && guild.OwnerID != uid {
		writeError(w, http.StatusForbidden, CodeForbidden, "only the author or guild owner can delete this message")
		return
	}

	if err := s.messages.Delete(r.Context(), rec); err != nil {
		writeStoreError(w, s.logger, err, "message not found")
		return
	}

	payload := map[string]string{
		"id":         rec.ID.String(),
		"channel_id": channel.ID.String(),
		"guild_id":   channel.GuildID.String(),
	}
	s.publishEvent(func() error {
		return s.publisher.ToGuild(channel.GuildID.String(), events.TypeMessageDelete, payload)
	}, events.TypeMessageDelete)

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTyping(w http.ResponseWriter, r *http.Request) {
	channel, _, uid, ok := s.channelContext(w, r)
	if !ok {
		return
	}
	users, err := s.userCache.GetByIDs(r.Context(), []gocql.UUID{uid})
	if err != nil {
		writeStoreError(w, s.logger, err, "user not found")
		return
	}
	payload := map[string]string{
		"channel_id": channel.ID.String(),
		"guild_id":   channel.GuildID.String(),
		"user_id":    uid.String(),
		"username":   users[uid].Username,
	}
	s.publishEvent(func() error {
		return s.publisher.ToGuild(channel.GuildID.String(), events.TypeTypingStart, payload)
	}, events.TypeTypingStart)

	w.WriteHeader(http.StatusNoContent)
}
