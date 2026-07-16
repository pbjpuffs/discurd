package httpapi

import (
	"net/http"

	"github.com/gocql/gocql"

	"discurd/internal/models"
	"discurd/internal/store"
)

// currentUser loads the authenticated user's row; writes the error response
// itself on failure.
func (s *Server) currentUser(w http.ResponseWriter, r *http.Request) (store.UserRecord, bool) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "not authenticated")
		return store.UserRecord{}, false
	}
	rec, err := s.users.GetByID(r.Context(), uid)
	if err != nil {
		writeStoreError(w, s.logger, err, "user not found")
		return store.UserRecord{}, false
	}
	return rec, true
}

func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	rec, ok := s.currentUser(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, rec.Model())
}

func (s *Server) handlePatchMe(w http.ResponseWriter, r *http.Request) {
	rec, ok := s.currentUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Username *string `json:"username"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Username != nil && *req.Username != rec.Username {
		if err := ValidateUsername(*req.Username); err != nil {
			writeError(w, http.StatusBadRequest, CodeValidationFailed, err.Error())
			return
		}
		if err := s.users.UpdateUsername(r.Context(), rec.ID, rec.Username, *req.Username); err != nil {
			writeStoreError(w, s.logger, err, "user not found")
			return
		}
		rec.Username = *req.Username
		s.userCache.Invalidate(rec.ID)
	}
	writeJSON(w, http.StatusOK, rec.Model())
}

func (s *Server) handleMyGuilds(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "not authenticated")
		return
	}
	ids, err := s.guilds.UserGuildIDs(r.Context(), uid)
	if err != nil {
		writeStoreError(w, s.logger, err, "guilds not found")
		return
	}
	recs, err := s.guilds.GetMany(r.Context(), ids)
	if err != nil {
		writeStoreError(w, s.logger, err, "guilds not found")
		return
	}
	out := make([]models.Guild, 0, len(recs))
	for _, g := range recs {
		out = append(out, g.Model())
	}
	writeJSON(w, http.StatusOK, out)
}

// requireMember checks membership after confirming the guild exists.
// It writes the error response itself and returns false on failure.
func (s *Server) requireMember(w http.ResponseWriter, r *http.Request, guildID, userID gocql.UUID) (store.GuildRecord, bool) {
	guild, err := s.guilds.Get(r.Context(), guildID)
	if err != nil {
		writeStoreError(w, s.logger, err, "guild not found")
		return store.GuildRecord{}, false
	}
	member, err := s.guilds.IsMember(r.Context(), guildID, userID)
	if err != nil {
		writeStoreError(w, s.logger, err, "guild not found")
		return store.GuildRecord{}, false
	}
	if !member {
		writeError(w, http.StatusForbidden, CodeForbidden, "you are not a member of this guild")
		return store.GuildRecord{}, false
	}
	return guild, true
}
