package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/gocql/gocql"

	"discurd/internal/auth"
	"discurd/internal/models"
	"discurd/internal/store"
)

type tokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type authResponse struct {
	User models.User `json:"user"`
	tokenPair
}

func (s *Server) issueTokens(w http.ResponseWriter, r *http.Request, userID string) (tokenPair, bool) {
	access, err := s.jwt.Issue(userID)
	if err != nil {
		s.logger.Error("issue access token", "error", err.Error())
		writeError(w, http.StatusInternalServerError, CodeInternal, "internal server error")
		return tokenPair{}, false
	}
	refresh, err := s.refresh.Issue(r.Context(), userID)
	if err != nil {
		s.logger.Error("issue refresh token", "error", err.Error())
		writeError(w, http.StatusInternalServerError, CodeInternal, "internal server error")
		return tokenPair{}, false
	}
	return tokenPair{AccessToken: access, RefreshToken: refresh}, true
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := ValidateUsername(req.Username); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, err.Error())
		return
	}
	email, err := NormalizeEmail(req.Email)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, err.Error())
		return
	}
	if err := ValidatePassword(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, err.Error())
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.logger.Error("hash password", "error", err.Error())
		writeError(w, http.StatusInternalServerError, CodeInternal, "internal server error")
		return
	}

	id, err := gocql.RandomUUID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "internal server error")
		return
	}
	rec := store.UserRecord{
		ID:           id,
		Username:     req.Username,
		Email:        email,
		PasswordHash: hash,
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.users.Create(r.Context(), rec); err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, CodeConflict, err.Error())
			return
		}
		writeStoreError(w, s.logger, err, "user not found")
		return
	}

	tokens, ok := s.issueTokens(w, r, id.String())
	if !ok {
		return
	}
	writeJSON(w, http.StatusCreated, authResponse{User: rec.Model(), tokenPair: tokens})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	email, err := NormalizeEmail(req.Email)
	if err != nil {
		writeError(w, http.StatusUnauthorized, CodeInvalidCredentials, "invalid email or password")
		return
	}

	rec, err := s.users.GetByEmail(r.Context(), email)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !auth.CheckPassword(rec.PasswordHash, req.Password)) {
		writeError(w, http.StatusUnauthorized, CodeInvalidCredentials, "invalid email or password")
		return
	}
	if err != nil {
		writeStoreError(w, s.logger, err, "user not found")
		return
	}

	tokens, ok := s.issueTokens(w, r, rec.ID.String())
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, authResponse{User: rec.Model(), tokenPair: tokens})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, "refresh_token is required")
		return
	}

	userID, newRefresh, err := s.refresh.Rotate(r.Context(), req.RefreshToken)
	if errors.Is(err, auth.ErrRefreshNotFound) {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "invalid refresh token")
		return
	}
	if err != nil {
		s.logger.Error("rotate refresh token", "error", err.Error())
		writeError(w, http.StatusInternalServerError, CodeInternal, "internal server error")
		return
	}
	access, err := s.jwt.Issue(userID)
	if err != nil {
		s.logger.Error("issue access token", "error", err.Error())
		writeError(w, http.StatusInternalServerError, CodeInternal, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, tokenPair{AccessToken: access, RefreshToken: newRefresh})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.RefreshToken != "" {
		if err := s.refresh.Revoke(r.Context(), req.RefreshToken); err != nil {
			s.logger.Error("revoke refresh token", "error", err.Error())
			writeError(w, http.StatusInternalServerError, CodeInternal, "internal server error")
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
